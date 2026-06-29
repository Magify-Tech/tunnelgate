package container

import "testing"

type testRepository interface {
	Value() string
}

type testSQLRepository struct{}

func (testSQLRepository) Value() string { return "ok" }

type testService struct {
	repo testRepository
}

type testHandler struct {
	Service *testService `container:""`
	Repo    testRepository
	Named   testRepository `container:"repo"`
}

func TestContainerRegistersAndResolvesBeans(t *testing.T) {
	container := NewContainer()
	repo := testSQLRepository{}
	if err := container.Register(repo, WithName("repo"), AsType((*testRepository)(nil))); err != nil {
		t.Fatalf("register repo: %v", err)
	}

	var resolved testRepository
	if err := container.Get(&resolved); err != nil {
		t.Fatalf("get repo: %v", err)
	}
	if resolved.Value() != "ok" {
		t.Fatalf("resolved repo returned %q", resolved.Value())
	}

	created, err := container.RegisterFactory(func(repo testRepository) *testService {
		return &testService{repo: repo}
	})
	if err != nil {
		t.Fatalf("register factory: %v", err)
	}
	service, ok := created.(*testService)
	if !ok {
		t.Fatalf("factory returned %T", created)
	}
	if service.repo.Value() != "ok" {
		t.Fatalf("service repo returned %q", service.repo.Value())
	}
}

func TestContainerAutowiresTaggedFields(t *testing.T) {
	container := NewContainer()
	repo := testSQLRepository{}
	if err := container.Register(repo, WithName("repo"), AsType((*testRepository)(nil))); err != nil {
		t.Fatalf("register repo: %v", err)
	}
	service := &testService{repo: repo}
	if err := container.Register(service); err != nil {
		t.Fatalf("register service: %v", err)
	}

	handler := &testHandler{}
	if err := container.Autowire(handler); err != nil {
		t.Fatalf("autowire: %v", err)
	}
	if handler.Service != service {
		t.Fatalf("handler service was not injected")
	}
	if handler.Named == nil || handler.Named.Value() != "ok" {
		t.Fatalf("handler named repo was not injected")
	}
	if handler.Repo != nil {
		t.Fatalf("untagged fields should not be injected")
	}
}

func TestContainerSupportsNameOnlyBeansWithDuplicateConcreteType(t *testing.T) {
	container := NewContainer()
	primary := "primary"
	secondary := "secondary"

	if err := container.Register(&primary, WithName("primary")); err != nil {
		t.Fatalf("register primary: %v", err)
	}
	if err := container.Register(&secondary, WithName("secondary"), WithNameOnly()); err != nil {
		t.Fatalf("register secondary: %v", err)
	}

	var resolved *string
	if err := container.GetNamed("secondary", &resolved); err != nil {
		t.Fatalf("get secondary: %v", err)
	}
	if resolved != &secondary {
		t.Fatalf("resolved name-only bean does not match")
	}

	if err := container.Get(&resolved); err != nil {
		t.Fatalf("get primary by type: %v", err)
	}
	if resolved != &primary {
		t.Fatalf("type lookup should still resolve primary")
	}
}

func TestContainerOptionalGetReturnsFalseForMissingBean(t *testing.T) {
	container := NewContainer()

	var service *testService
	ok, err := container.GetOptional(&service)
	if err != nil {
		t.Fatalf("optional get returned error: %v", err)
	}
	if ok {
		t.Fatalf("optional get should report missing bean")
	}

	ok, err = container.GetNamedOptional("missing", &service)
	if err != nil {
		t.Fatalf("optional named get returned error: %v", err)
	}
	if ok {
		t.Fatalf("optional named get should report missing bean")
	}
}
