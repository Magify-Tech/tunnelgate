package container

import (
	"testing"

	"postman-transform/backend-golang/pkg/pluginpkg"
)

type loaderModule struct {
	Repo        testRepository `container:""`
	initialized bool
}

func (m *loaderModule) Init(*Container) error {
	m.initialized = m.Repo != nil
	return nil
}

func (m *loaderModule) Name() string {
	return "loader-module"
}

func TestLoaderContainerRegistrarFunction(t *testing.T) {
	container := NewContainer()
	loader := NewLoader(container)

	err := loader.callContainerRegistrarSymbol(func(c *Container) error {
		return c.Register(testSQLRepository{}, AsType((*testRepository)(nil)))
	})
	if err != nil {
		t.Fatalf("call container registrar: %v", err)
	}

	var repo testRepository
	if err := container.Get(&repo); err != nil {
		t.Fatalf("get registered repo: %v", err)
	}
	if repo.Value() != "ok" {
		t.Fatalf("repo returned %q", repo.Value())
	}
}

func TestLoaderContainerRegistrarFunctionAcceptsPublicContainerInterface(t *testing.T) {
	container := NewContainer()
	loader := NewLoader(container)

	err := loader.callContainerRegistrarSymbol(func(c pluginpkg.ServiceContainer) error {
		return c.Register(testSQLRepository{}, pluginpkg.AsType((*testRepository)(nil)))
	})
	if err != nil {
		t.Fatalf("call public container registrar: %v", err)
	}

	var repo testRepository
	if err := container.Get(&repo); err != nil {
		t.Fatalf("get registered repo: %v", err)
	}
	if repo.Value() != "ok" {
		t.Fatalf("repo returned %q", repo.Value())
	}
}

func TestLoaderWireModuleAutowiresInitializesAndRegisters(t *testing.T) {
	container := NewContainer()
	if err := container.Register(testSQLRepository{}, AsType((*testRepository)(nil))); err != nil {
		t.Fatalf("register repo: %v", err)
	}
	loader := NewLoader(container)
	module := &loaderModule{}

	if err := loader.wireModule(module); err != nil {
		t.Fatalf("wire module: %v", err)
	}
	if module.Repo == nil {
		t.Fatalf("module repo was not injected")
	}
	if !module.initialized {
		t.Fatalf("module was not initialized")
	}

	var resolved *loaderModule
	if err := container.GetNamed("loader-module", &resolved); err != nil {
		t.Fatalf("get module container entry: %v", err)
	}
	if resolved != module {
		t.Fatalf("resolved module does not match")
	}
}

func TestLoaderLoadInstanceAndConfigure(t *testing.T) {
	container := NewContainer()
	if err := container.Register(testSQLRepository{}, AsType((*testRepository)(nil))); err != nil {
		t.Fatalf("register repo: %v", err)
	}
	loader := NewLoader(container)
	module := &loaderModule{}

	loaded, err := loader.LoadInstance("testModule", module)
	if err != nil {
		t.Fatalf("load instance: %v", err)
	}
	if loaded.Instance != module {
		t.Fatalf("loaded instance does not match")
	}
	if module.Repo == nil {
		t.Fatalf("module repo was not injected")
	}
}

func TestModuleFromSymbolReturnsPointerToVariable(t *testing.T) {
	module := loaderModule{}
	instance, err := moduleFromSymbol(&module)
	if err != nil {
		t.Fatalf("module from symbol: %v", err)
	}
	if instance != &module {
		t.Fatalf("expected pointer to module variable, got %T", instance)
	}
}
