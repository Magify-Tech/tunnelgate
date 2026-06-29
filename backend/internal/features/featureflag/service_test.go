package featureflag

import "testing"

func TestServiceNormalizesFeatureList(t *testing.T) {
	service := NewService()

	service.Put([]string{" proxy ", "api-specs", "proxy", ""})

	got := service.Get()
	want := []string{"api-specs", "proxy"}
	if len(got) != len(want) {
		t.Fatalf("Get returned %v, want %v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("Get returned %v, want %v", got, want)
		}
	}
}

func TestServiceGetReturnsCopy(t *testing.T) {
	service := NewService()
	service.Put([]string{"api-specs"})

	got := service.Get()
	got[0] = "changed"

	again := service.Get()
	if again[0] != "api-specs" {
		t.Fatalf("Get exposed internal slice: %v", again)
	}
}
