package selenium

import (
	"reflect"
	"testing"
)

func TestValidateCapabilities(t *testing.T) {
	c := Capabilities{
		browserVersion: "100.0",
		platformName:   "linux",
	}
	c.ValidateCapabilities()
	if c[version] != "100.0" {
		t.Errorf("expected version set from browserVersion")
	}
	if c[platform] != "linux" {
		t.Errorf("expected platform set from platformName")
	}
}

func TestProcessCapabilitiesLegacyDesiredCaps(t *testing.T) {
	c := Capabilities{
		desiredCapabilities: map[string]any{
			browserName:    "chrome",
			browserVersion: "101",
		},
	}
	result, err := c.ProcessCapabilities()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result[browserName] != "chrome" || result[browserVersion] != "101" {
		t.Errorf("unexpected result: %+v", result)
	}
}

func TestProcessCapabilitiesW3CAlwaysMatchFirstMatch(t *testing.T) {
	c := Capabilities{
		capabilities: map[string]any{
			alwaysMatch: map[string]any{
				browserName:    "firefox",
				browserVersion: "102",
			},
			firstMatch: []any{
				map[string]any{
					browserVersion: "override",
				},
			},
		},
	}
	result, err := c.ProcessCapabilities()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result[browserName] != "firefox" || result[browserVersion] != "override" {
		t.Errorf("merge failed: %+v", result)
	}
}

func TestProcessCapabilitiesErrorNoValid(t *testing.T) {
	c := Capabilities{
		capabilities: map[string]any{
			firstMatch: []any{
				map[string]any{"something": "else"},
			},
		},
	}
	_, err := c.ProcessCapabilities()
	if err == nil {
		t.Fatal("expected error for no valid capabilities")
	}
}

func TestGetBrowserNameVariants(t *testing.T) {
	c := Capabilities{browserName: "chrome"}
	if got := c.GetBrowserName(); got != "chrome" {
		t.Errorf("expected chrome, got %s", got)
	}
	c2 := Capabilities{deviceName: "android"}
	if got := c2.GetBrowserName(); got != "android" {
		t.Errorf("expected android, got %s", got)
	}
	c3 := Capabilities{}
	if got := c3.GetBrowserName(); got != "" {
		t.Errorf("expected empty, got %s", got)
	}
}

func TestGetBrowserVersionVariants(t *testing.T) {
	c := Capabilities{browserVersion: "110"}
	if got := c.GetBrowserVersion(); got != "110" {
		t.Errorf("expected 110, got %s", got)
	}
	c2 := Capabilities{version: "legacy"}
	if got := c2.GetBrowserVersion(); got != "legacy" {
		t.Errorf("expected legacy, got %s", got)
	}
	c3 := Capabilities{}
	if got := c3.GetBrowserVersion(); got != "" {
		t.Errorf("expected empty, got %s", got)
	}
}

func TestGetCapabilityAllBranches(t *testing.T) {
	c := Capabilities{
		"foo": "bar",
		desiredCapabilities: map[string]any{
			"dc": "yes",
		},
		capabilities: map[string]any{
			alwaysMatch: map[string]any{
				"am": "always",
			},
			firstMatch: []any{
				map[string]any{"fm": "first"},
			},
		},
	}
	if got := c.GetCapability("foo"); got != "bar" {
		t.Errorf("expected foo=bar, got %v", got)
	}
	if got := c.GetCapability("dc"); got != "yes" {
		t.Errorf("expected dc=yes, got %v", got)
	}
	if got := c.GetCapability("am"); got != "always" {
		t.Errorf("expected am=always, got %v", got)
	}
	if got := c.GetCapability("fm"); got != "first" {
		t.Errorf("expected fm=first, got %v", got)
	}
	if got := c.GetCapability("unknown"); got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

func TestRemoveCapabilityAllBranches(t *testing.T) {
	c := Capabilities{
		"foo":               map[string]any{"foo": "bar"},
		desiredCapabilities: map[string]any{"baz": "qux"},
		capabilities: map[string]any{
			alwaysMatch: map[string]any{"alpha": "beta"},
			firstMatch: []any{
				map[string]any{"gamma": "delta"},
			},
		},
	}
	c.RemoveCapability("foo")
	c.RemoveCapability("baz")
	c.RemoveCapability("alpha")
	c.RemoveCapability("gamma")
	if _, ok := c["foo"].(map[string]any)["foo"]; ok {
		t.Error("foo not removed")
	}
	if _, ok := c[desiredCapabilities].(map[string]any)["baz"]; ok {
		t.Error("baz not removed")
	}
	am := c[capabilities].(map[string]any)[alwaysMatch].(map[string]any)
	if _, ok := am["alpha"]; ok {
		t.Error("alpha not removed")
	}
	fm := c[capabilities].(map[string]any)[firstMatch].([]any)[0].(map[string]any)
	if _, ok := fm["gamma"]; ok {
		t.Error("gamma not removed")
	}
}

func TestDeepCopyPrimitivesAndNested(t *testing.T) {
	orig := Capabilities{
		"str": "val",
		"map": map[string]any{"key": "val"},
		"slice": []any{
			map[string]any{"nested": "yes"},
		},
		"caps": Capabilities{"nestedcap": "ok"},
	}
	cp := orig.DeepCopy()
	if !reflect.DeepEqual(orig, cp) {
		t.Errorf("deep copy mismatch: %+v vs %+v", orig, cp)
	}
	if &orig == &cp {
		t.Errorf("expected different map pointers")
	}
}
