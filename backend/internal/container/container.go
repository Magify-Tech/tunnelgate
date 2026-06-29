package container

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"

	"postman-transform/backend-golang/pkg/pluginpkg"
)

var (
	ErrBeanNotFound  = errors.New("bean not found")
	ErrBeanAmbiguous = errors.New("multiple beans match requested type")
)

type beanEntry struct {
	name  string
	value reflect.Value
	typ   reflect.Type
}

// ContainerOption customizes how an entry is registered in the Container.
type ContainerOption = pluginpkg.ContainerOption

// BeanOption is kept for compatibility with older plugin code.
type BeanOption = pluginpkg.BeanOption

// WithName registers a container entry under a stable name for named injection.
func WithName(name string) ContainerOption {
	return pluginpkg.WithName(name)
}

// WithNameOnly skips concrete type registration. Use it when multiple entries
// share a concrete type and should only be retrieved by name.
func WithNameOnly() ContainerOption {
	return pluginpkg.WithNameOnly()
}

// AsType registers a container entry under another assignable type, usually an interface:
//
//	container.Register(sqlRepo, container.AsType((*Repository)(nil)))
func AsType(typeToken any) ContainerOption {
	return pluginpkg.AsType(typeToken)
}

// Container is a minimal registry with constructor and field injection.
type Container struct {
	mu     sync.RWMutex
	byType map[reflect.Type]beanEntry
	byName map[string]beanEntry
}

var _ pluginpkg.ServiceContainer = (*Container)(nil)

// NewContainer creates a Container and registers the container itself.
func NewContainer() *Container {
	c := &Container{
		byType: make(map[reflect.Type]beanEntry),
		byName: make(map[string]beanEntry),
	}
	_ = c.Register(c, WithName("container"))
	return c
}

func (c *Container) List() []string {
	keys := make([]string, len(c.byName))

	i := 0
	for k := range c.byName {
		keys[i] = k
		i++
	}
	return keys
}

// Register adds a concrete container entry. By default the entry is registered under its
// concrete type. Use AsType to also register it by an interface.
func (c *Container) Register(container any, opts ...ContainerOption) error {
	if c == nil {
		return errors.New("container is nil")
	}
	if container == nil {
		return errors.New("container entry cannot be nil")
	}
	options := pluginpkg.DefaultContainerOptions()
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		if err := opt(&options); err != nil {
			return err
		}
	}

	value := reflect.ValueOf(container)
	if isNilValue(value) {
		return errors.New("container entry cannot be nil")
	}
	entry := beanEntry{value: value, typ: value.Type(), name: options.Name}

	types := []reflect.Type{}
	if options.RegisterConcrete {
		types = append(types, entry.typ)
	}
	for _, alias := range options.As {
		if !typeAssignableTo(entry.typ, alias) {
			return fmt.Errorf("container entry type %s is not assignable to %s", entry.typ, alias)
		}
		types = append(types, alias)
	}
	if len(types) == 0 && entry.name == "" {
		return errors.New("container entry must be registered by type, alias, or name")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	for _, typ := range types {
		if err := c.ensureTypeAvailableLocked(typ); err != nil {
			return err
		}
	}
	if entry.name != "" {
		if _, exists := c.byName[entry.name]; exists {
			return fmt.Errorf("container entry name %q already registered", entry.name)
		}
	}
	for _, typ := range types {
		c.byType[typ] = entry
	}
	if entry.name != "" {
		c.byName[entry.name] = entry
	}
	return nil
}

// RegisterFactory invokes a constructor with arguments resolved from the
// container, then registers the first non-error return value as a bean.
func (c *Container) RegisterFactory(factory any, opts ...ContainerOption) (any, error) {
	container, err := c.New(factory)
	if err != nil {
		return nil, err
	}
	if err := c.Register(container, opts...); err != nil {
		return nil, err
	}
	return container, nil
}

// New invokes a constructor with container-managed arguments and returns the
// first non-error result without registering it.
func (c *Container) New(constructor any) (any, error) {
	results, err := c.Invoke(constructor)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, errors.New("constructor returned no value")
	}
	return results[0], nil
}

// Invoke calls a function with each argument resolved by type from the
// container. Return values assignable to error are handled as errors and are
// not included in the returned slice.
func (c *Container) Invoke(fn any) ([]any, error) {
	if c == nil {
		return nil, errors.New("container is nil")
	}
	if fn == nil {
		return nil, errors.New("function cannot be nil")
	}
	value := reflect.ValueOf(fn)
	if value.Kind() != reflect.Func {
		return nil, fmt.Errorf("expected function, got %T", fn)
	}
	valueType := value.Type()
	args := make([]reflect.Value, valueType.NumIn())
	for i := 0; i < valueType.NumIn(); i++ {
		arg, err := c.ResolveType(valueType.In(i))
		if err != nil {
			return nil, fmt.Errorf("resolve argument %d (%s): %w", i, valueType.In(i), err)
		}
		args[i] = arg
	}

	rawResults := value.Call(args)
	results := make([]any, 0, len(rawResults))
	for _, result := range rawResults {
		if typeAssignableTo(result.Type(), errorType) {
			if !isNilValue(result) {
				return results, result.Interface().(error)
			}
			continue
		}
		results = append(results, result.Interface())
	}
	return results, nil
}

// Get resolves a bean into a pointer target:
//
//	var service *Service
//	err := container.Get(&service)
func (c *Container) Get(target any) error {
	if target == nil {
		return errors.New("target cannot be nil")
	}
	targetValue := reflect.ValueOf(target)
	if targetValue.Kind() != reflect.Pointer || targetValue.IsNil() {
		return errors.New("target must be a non-nil pointer")
	}
	elem := targetValue.Elem()
	if !elem.CanSet() {
		return errors.New("target cannot be set")
	}
	value, err := c.ResolveType(elem.Type())
	if err != nil {
		return err
	}
	elem.Set(value)
	return nil
}

// GetOptional resolves a bean into target. It returns false when the bean is
// not registered and still returns other resolution errors, such as ambiguity.
func (c *Container) GetOptional(target any) (bool, error) {
	if err := c.Get(target); err != nil {
		if errors.Is(err, ErrBeanNotFound) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// GetNamed resolves a bean by name into a pointer target.
func (c *Container) GetNamed(name string, target any) error {
	if target == nil {
		return errors.New("target cannot be nil")
	}
	value, err := c.ResolveName(name)
	if err != nil {
		return err
	}
	targetValue := reflect.ValueOf(target)
	if targetValue.Kind() != reflect.Pointer || targetValue.IsNil() {
		return errors.New("target must be a non-nil pointer")
	}
	elem := targetValue.Elem()
	if !typeAssignableTo(value.Type(), elem.Type()) {
		return fmt.Errorf("named bean %q has type %s, not assignable to %s", name, value.Type(), elem.Type())
	}
	elem.Set(value)
	return nil
}

// GetNamedOptional resolves a named bean into target. It returns false when
// the name is not registered.
func (c *Container) GetNamedOptional(name string, target any) (bool, error) {
	if err := c.GetNamed(name, target); err != nil {
		if errors.Is(err, ErrBeanNotFound) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// Autowire injects exported struct fields tagged with `container`, `bean`, or `inject`.
// Empty tag values resolve by field type; non-empty values resolve by name.
func (c *Container) Autowire(target any) error {
	if target == nil {
		return errors.New("target cannot be nil")
	}
	targetValue := reflect.ValueOf(target)
	if targetValue.Kind() != reflect.Pointer || targetValue.IsNil() {
		return errors.New("target must be a non-nil pointer to struct")
	}
	structValue := targetValue.Elem()
	if structValue.Kind() != reflect.Struct {
		return errors.New("target must be a pointer to struct")
	}
	structType := structValue.Type()
	for i := 0; i < structValue.NumField(); i++ {
		fieldInfo := structType.Field(i)
		tag, ok := injectionTag(fieldInfo)
		if !ok {
			continue
		}
		field := structValue.Field(i)
		if !field.CanSet() {
			return fmt.Errorf("field %s cannot be injected", fieldInfo.Name)
		}
		var (
			value reflect.Value
			err   error
		)
		if tag == "" {
			value, err = c.ResolveType(field.Type())
		} else {
			value, err = c.ResolveName(tag)
			if err == nil && !typeAssignableTo(value.Type(), field.Type()) {
				err = fmt.Errorf("named bean %q has type %s, not assignable to field %s (%s)", tag, value.Type(), fieldInfo.Name, field.Type())
			}
		}
		if err != nil {
			return fmt.Errorf("inject field %s: %w", fieldInfo.Name, err)
		}
		field.Set(value)
	}
	return nil
}

// ResolveType returns a bean value assignable to typ.
func (c *Container) ResolveType(typ reflect.Type) (reflect.Value, error) {
	if c == nil {
		return reflect.Value{}, errors.New("container is nil")
	}
	if typ == nil {
		return reflect.Value{}, errors.New("type cannot be nil")
	}
	c.mu.RLock()
	if entry, ok := c.byType[typ]; ok {
		c.mu.RUnlock()
		return assignValue(entry.value, typ)
	}

	var matches []beanEntry
	seen := make(map[reflect.Type]struct{})
	for registeredType, entry := range c.byType {
		if registeredType == typ {
			continue
		}
		if _, ok := seen[entry.typ]; ok {
			continue
		}
		if typeAssignableTo(registeredType, typ) || typeAssignableTo(entry.value.Type(), typ) {
			matches = append(matches, entry)
			seen[entry.typ] = struct{}{}
		}
	}
	c.mu.RUnlock()

	switch len(matches) {
	case 0:
		return reflect.Value{}, fmt.Errorf("%w: %s", ErrBeanNotFound, typ)
	case 1:
		return assignValue(matches[0].value, typ)
	default:
		return reflect.Value{}, fmt.Errorf("%w: %s", ErrBeanAmbiguous, typ)
	}
}

// ResolveName returns a bean value by name.
func (c *Container) ResolveName(name string) (reflect.Value, error) {
	if c == nil {
		return reflect.Value{}, errors.New("container is nil")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return reflect.Value{}, errors.New("bean name cannot be empty")
	}
	c.mu.RLock()
	entry, ok := c.byName[name]
	c.mu.RUnlock()
	if !ok {
		return reflect.Value{}, fmt.Errorf("%w: %q", ErrBeanNotFound, name)
	}
	return entry.value, nil
}

func (c *Container) ensureTypeAvailableLocked(typ reflect.Type) error {
	if _, exists := c.byType[typ]; exists {
		return fmt.Errorf("bean type %s already registered", typ)
	}
	return nil
}

func injectionTag(field reflect.StructField) (string, bool) {
	if tag, ok := field.Tag.Lookup("container"); ok {
		if tag == "-" {
			return "", false
		}
		return strings.TrimSpace(tag), true
	}
	if tag, ok := field.Tag.Lookup("bean"); ok {
		if tag == "-" {
			return "", false
		}
		return strings.TrimSpace(tag), true
	}
	if tag, ok := field.Tag.Lookup("inject"); ok {
		if tag == "-" {
			return "", false
		}
		return strings.TrimSpace(tag), true
	}
	return "", false
}

func assignValue(value reflect.Value, typ reflect.Type) (reflect.Value, error) {
	if typeAssignableTo(value.Type(), typ) {
		return value, nil
	}
	return reflect.Value{}, fmt.Errorf("bean type %s is not assignable to %s", value.Type(), typ)
}

func typeAssignableTo(source reflect.Type, target reflect.Type) bool {
	if source.AssignableTo(target) {
		return true
	}
	return target.Kind() == reflect.Interface && source.Implements(target)
}

func isNilValue(value reflect.Value) bool {
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

var errorType = reflect.TypeOf((*error)(nil)).Elem()
