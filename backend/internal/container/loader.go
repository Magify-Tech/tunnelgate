package container

import (
	"errors"
	"fmt"
	goplugin "plugin"
	"reflect"

	"postman-transform/backend-golang/pkg/pluginpkg"
)

const (
	defaultModuleSymbol        = "Module"
	defaultModuleFactorySymbol = "NewModule"
)

// ContainerRegistrar can be implemented by a plugin symbol to register additional
// dependencies before the module itself is built.
type ContainerRegistrar = pluginpkg.ContainerRegistrar

type legacyPublicBeanRegistrar = pluginpkg.BeanRegistrar

type legacyBeanRegistrar interface {
	RegisterBeans(*Container) error
}

// Initializer can be implemented by a module after its bean fields are wired.
type Initializer interface {
	Init(*Container) error
}

// Loader imports Go plugins and wires their dependencies through a Container.
type Loader struct {
	Container            *Container
	ModuleSymbols        []string
	ModuleFactorySymbols []string
	ContainerSymbols     []string
	BeanRegistrarSymbols []string
	RegisterModuleBean   bool
}

// LoadedModule is a loaded plugin module instance plus its backing plugin.
type LoadedModule struct {
	Path      string
	Symbol    string
	Plugin    *goplugin.Plugin
	Instance  any
	Container *Container
}

// NewLoader creates a plugin loader. If container is nil, a new one is used.
func NewLoader(container *Container) *Loader {
	if container == nil {
		container = NewContainer()
	}
	return &Loader{
		Container:            container,
		ModuleSymbols:        []string{defaultModuleSymbol},
		ModuleFactorySymbols: []string{defaultModuleFactorySymbol},
		ContainerSymbols:     []string{"RegisterContainers", "RegisterBeans", "Containers", "Beans"},
		RegisterModuleBean:   true,
	}
}

// Load imports a .so plugin, calls optional container registration symbols, creates
// or reads the module instance, autowires it, initializes it, and optionally
// registers it as a container entry.
func (l *Loader) Load(path string) (*LoadedModule, error) {
	if l == nil {
		return nil, errors.New("loader is nil")
	}
	if l.Container == nil {
		l.Container = NewContainer()
	}
	plug, err := goplugin.Open(path)
	if err != nil {
		return nil, err
	}
	if err := l.callContainerRegistrars(plug); err != nil {
		return nil, err
	}

	instance, symbol, err := l.loadModuleInstance(plug)
	if err != nil {
		return nil, err
	}
	if err := l.wireModule(instance); err != nil {
		return nil, err
	}

	loaded := &LoadedModule{
		Path:      path,
		Symbol:    symbol,
		Plugin:    plug,
		Instance:  instance,
		Container: l.Container,
	}
	return loaded, nil
}

// LoadAndConfigure loads a plugin and mounts its routes on router.
func (l *Loader) LoadAndConfigure(path string, router any) (*LoadedModule, error) {
	loaded, err := l.Load(path)
	if err != nil {
		return nil, err
	}
	if err := loaded.ConfigureRoutes(router); err != nil {
		return nil, err
	}
	return loaded, nil
}

// LoadInstance wires an already-linked module instance through the same path
// used for plugin modules. This is useful for built-in modules and tests.
func (l *Loader) LoadInstance(symbol string, instance any) (*LoadedModule, error) {
	if l == nil {
		return nil, errors.New("loader is nil")
	}
	if l.Container == nil {
		l.Container = NewContainer()
	}
	if symbol == "" {
		symbol = "Module"
	}
	if err := l.wireModule(instance); err != nil {
		return nil, err
	}
	return &LoadedModule{
		Symbol:    symbol,
		Instance:  instance,
		Container: l.Container,
	}, nil
}

// LoadInstanceAndConfigure wires an already-linked module instance and mounts
// its routes on router.
func (l *Loader) LoadInstanceAndConfigure(symbol string, instance any, router any) (*LoadedModule, error) {
	loaded, err := l.LoadInstance(symbol, instance)
	if err != nil {
		return nil, err
	}
	if err := loaded.ConfigureRoutes(router); err != nil {
		return nil, err
	}
	return loaded, nil
}

// ConfigureRoutes mounts the loaded module onto either *gin.Engine or
// *gin.RouterGroup.
func (m *LoadedModule) ConfigureRoutes(router any) error {
	if m == nil {
		return errors.New("loaded module is nil")
	}
	return ConfigureRoutes(m.Instance, router)
}

func (l *Loader) callContainerRegistrars(plug *goplugin.Plugin) error {
	for _, symbolName := range l.containerRegistrarSymbols() {
		symbol, ok := lookupSymbol(plug, symbolName)
		if !ok {
			continue
		}
		if err := l.callContainerRegistrarSymbol(symbol); err != nil {
			return fmt.Errorf("call %s: %w", symbolName, err)
		}
	}
	return nil
}

func (l *Loader) callContainerRegistrarSymbol(symbol any) error {
	if registrar, ok := symbol.(ContainerRegistrar); ok {
		return registrar.RegisterContainers(l.Container)
	}
	if registrar, ok := symbol.(legacyPublicBeanRegistrar); ok {
		return registrar.RegisterBeans(l.Container)
	}
	if registrar, ok := symbol.(legacyBeanRegistrar); ok {
		return registrar.RegisterBeans(l.Container)
	}
	if err := callRegisterContainersFunction(l.Container, reflect.ValueOf(symbol)); err == nil {
		return nil
	} else if !errors.Is(err, errUnsupportedSymbol) {
		return err
	}

	value := unwrapPluginVariable(reflect.ValueOf(symbol))
	if value.IsValid() {
		if value.CanInterface() {
			if registrar, ok := value.Interface().(ContainerRegistrar); ok {
				return registrar.RegisterContainers(l.Container)
			}
			if registrar, ok := value.Interface().(legacyPublicBeanRegistrar); ok {
				return registrar.RegisterBeans(l.Container)
			}
			if registrar, ok := value.Interface().(legacyBeanRegistrar); ok {
				return registrar.RegisterBeans(l.Container)
			}
		}
		if value.CanAddr() && value.Addr().CanInterface() {
			if registrar, ok := value.Addr().Interface().(ContainerRegistrar); ok {
				return registrar.RegisterContainers(l.Container)
			}
			if registrar, ok := value.Addr().Interface().(legacyPublicBeanRegistrar); ok {
				return registrar.RegisterBeans(l.Container)
			}
			if registrar, ok := value.Addr().Interface().(legacyBeanRegistrar); ok {
				return registrar.RegisterBeans(l.Container)
			}
		}
		if err := callRegisterContainersFunction(l.Container, value); err == nil {
			return nil
		} else if !errors.Is(err, errUnsupportedSymbol) {
			return err
		}
	}
	return fmt.Errorf("unsupported container registrar symbol type %T", symbol)
}

func (l *Loader) loadModuleInstance(plug *goplugin.Plugin) (any, string, error) {
	for _, symbolName := range l.moduleFactorySymbols() {
		symbol, ok := lookupSymbol(plug, symbolName)
		if !ok {
			continue
		}
		instance, err := l.Container.New(symbol)
		if err != nil {
			return nil, "", fmt.Errorf("call %s: %w", symbolName, err)
		}
		return instance, symbolName, nil
	}
	for _, symbolName := range l.moduleSymbols() {
		symbol, ok := lookupSymbol(plug, symbolName)
		if !ok {
			continue
		}
		instance, err := moduleFromSymbol(symbol)
		if err != nil {
			return nil, "", fmt.Errorf("read %s: %w", symbolName, err)
		}
		return instance, symbolName, nil
	}
	return nil, "", fmt.Errorf("plugin has no module factory %v or module symbol %v", l.moduleFactorySymbols(), l.moduleSymbols())
}

func (l *Loader) wireModule(instance any) error {
	if instance == nil {
		return errors.New("module instance is nil")
	}
	if shouldAutowire(instance) {
		if err := l.Container.Autowire(instance); err != nil {
			return err
		}
	}
	if initializer, ok := instance.(Initializer); ok {
		if err := initializer.Init(l.Container); err != nil {
			return err
		}
	} else if initializer, ok := instance.(pluginpkg.Initializer); ok {
		if err := initializer.Init(l.Container); err != nil {
			return err
		}
	}
	if l.RegisterModuleBean {
		opts := []ContainerOption{WithName(moduleBeanName(instance))}
		if err := l.Container.Register(instance, opts...); err != nil {
			return err
		}
	}
	return nil
}

func (l *Loader) moduleSymbols() []string {
	if len(l.ModuleSymbols) == 0 {
		return []string{defaultModuleSymbol}
	}
	return l.ModuleSymbols
}

func (l *Loader) moduleFactorySymbols() []string {
	if len(l.ModuleFactorySymbols) == 0 {
		return []string{defaultModuleFactorySymbol}
	}
	return l.ModuleFactorySymbols
}

func (l *Loader) containerRegistrarSymbols() []string {
	if len(l.ContainerSymbols) > 0 {
		return l.ContainerSymbols
	}
	if len(l.BeanRegistrarSymbols) > 0 {
		return l.BeanRegistrarSymbols
	}
	return []string{"RegisterContainers", "RegisterBeans", "Containers", "Beans"}
}

func lookupSymbol(plug *goplugin.Plugin, name string) (any, bool) {
	symbol, err := plug.Lookup(name)
	if err != nil {
		return nil, false
	}
	return symbol, true
}

func callRegisterContainersFunction(container *Container, value reflect.Value) error {
	if !value.IsValid() || value.Kind() != reflect.Func {
		return errUnsupportedSymbol
	}
	valueType := value.Type()
	if valueType.NumIn() != 1 || !typeAssignableTo(reflect.TypeOf(container), valueType.In(0)) {
		return errUnsupportedSymbol
	}
	if valueType.NumOut() > 1 {
		return fmt.Errorf("container registration function must return nothing or error")
	}
	if valueType.NumOut() == 1 && !typeAssignableTo(valueType.Out(0), errorType) {
		return fmt.Errorf("container registration function return type must be error")
	}
	results := value.Call([]reflect.Value{reflect.ValueOf(container)})
	if len(results) == 1 && !isNilValue(results[0]) {
		return results[0].Interface().(error)
	}
	return nil
}

func moduleFromSymbol(symbol any) (any, error) {
	value := unwrapPluginVariable(reflect.ValueOf(symbol))
	if !value.IsValid() || !value.CanInterface() {
		return nil, errors.New("module symbol cannot be read")
	}
	if isNilValue(value) {
		return nil, errors.New("module symbol is nil")
	}
	if value.Kind() != reflect.Pointer && value.CanAddr() {
		value = value.Addr()
	}
	instance := value.Interface()
	if instance == nil {
		return nil, errors.New("module symbol is nil")
	}
	return instance, nil
}

func unwrapPluginVariable(value reflect.Value) reflect.Value {
	if !value.IsValid() {
		return value
	}
	for {
		if value.Kind() == reflect.Pointer && !value.IsNil() && value.Elem().IsValid() && value.Elem().CanInterface() {
			value = value.Elem()
			continue
		}
		if value.Kind() == reflect.Interface && !value.IsNil() && value.Elem().IsValid() {
			value = value.Elem()
			continue
		}
		return value
	}
}

func moduleBeanName(instance any) string {
	if named, ok := instance.(interface{ Name() string }); ok {
		if name := named.Name(); name != "" {
			return name
		}
	}
	typ := reflect.TypeOf(instance)
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ.Name() != "" {
		return typ.Name()
	}
	return "module"
}

var errUnsupportedSymbol = errors.New("unsupported symbol")

func shouldAutowire(instance any) bool {
	value := reflect.ValueOf(instance)
	return value.IsValid() && value.Kind() == reflect.Pointer && !value.IsNil() && value.Elem().Kind() == reflect.Struct
}
