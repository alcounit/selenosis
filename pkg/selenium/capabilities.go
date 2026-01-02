package selenium

import (
	"fmt"

	"dario.cat/mergo"
)

var (
	browserName         = "browserName"
	browserVersion      = "browserVersion"
	version             = "version"
	capabilities        = "capabilities"
	desiredCapabilities = "desiredCapabilities"
	alwaysMatch         = "alwaysMatch"
	firstMatch          = "firstMatch"
	deviceName          = "deviceName"
	platformName        = "platformName"
	platform            = "platform"
)

type Capabilities map[string]any

func (c Capabilities) ValidateCapabilities() {
	if _, ok := c[browserVersion]; ok {
		c[version] = c[browserVersion]
	}

	if _, ok := c[platformName]; ok {
		c[platform] = c[platformName]
	}
}

func (c Capabilities) ProcessCapabilities() (Capabilities, error) {
	var (
		baseCaps   Capabilities
		alwaysCaps Capabilities
		firstCaps  []Capabilities
	)

	if dc, ok := c[desiredCapabilities].(map[string]any); ok {
		baseCaps = Capabilities(dc).DeepCopy()
	}

	if rawCaps, ok := c[capabilities].(map[string]any); ok {
		if ac, ok := rawCaps[alwaysMatch].(map[string]any); ok {
			alwaysCaps = Capabilities(ac).DeepCopy()
		}
		if fm, ok := rawCaps[firstMatch].([]any); ok {
			for _, f := range fm {
				if fmMap, ok := f.(map[string]any); ok {
					firstCaps = append(firstCaps, Capabilities(fmMap).DeepCopy())
				}
			}
		}
	}

	if alwaysCaps != nil && (baseCaps == nil || baseCaps[browserName] == nil) {
		baseCaps = alwaysCaps
	}

	if len(firstCaps) == 0 {
		firstCaps = []Capabilities{{}}
	}

	for _, fm := range firstCaps {
		merged := baseCaps.DeepCopy()

		if err := mergo.Merge(&merged, fm.DeepCopy(), mergo.WithOverride); err != nil {
			return nil, fmt.Errorf("merge capabilities: %w", err)
		}

		result := Capabilities{}
		if bn, ok := merged[browserName].(string); ok {
			result[browserName] = bn
		}
		if bv, ok := merged[browserVersion].(string); ok {
			result[browserVersion] = bv
		} else if v, ok := merged[version].(string); ok {
			result[browserVersion] = v
		}

		if len(result) > 0 {
			return result, nil
		}
	}

	return nil, fmt.Errorf("no valid capabilities found")
}

func (c Capabilities) GetBrowserName() string {
	browserName, ok := c[browserName]
	if ok {
		return browserName.(string)
	}

	deviceName, ok := c["deviceName"]
	if !ok {
		return ""
	}
	return deviceName.(string)
}

func (c Capabilities) GetBrowserVersion() string {
	if bv, ok := c[browserVersion]; ok {
		return bv.(string)
	}
	if v, ok := c[version]; ok {
		return v.(string)
	}
	return ""
}

func (c Capabilities) GetCapability(capabilityName string) any {
	if raw, ok := c[capabilityName]; ok {
		return raw
	}

	if raw, ok := c[desiredCapabilities]; ok {
		if dc, ok := raw.(map[string]any); ok {
			if raw, ok := dc[capabilityName]; ok {
				return raw
			}
		}
	}

	if raw, ok := c[capabilities]; ok {
		if c, ok := raw.(map[string]any); ok {
			if raw, ok := c[alwaysMatch]; ok {
				if am, ok := raw.(map[string]any); ok {
					if raw, ok := am[capabilityName]; ok {
						return raw
					}
				}
			}
			if raw, ok := c[firstMatch]; ok {
				if fm, ok := raw.([]any); ok {
					for _, raw := range fm {
						if c, ok := raw.(map[string]any); ok {
							if raw, ok := c[capabilityName]; ok {
								return raw
							}
						}
					}
				}
			}
		}
	}

	return nil
}

func (c Capabilities) RemoveCapability(capabilityName string) {

	if raw, ok := c[capabilityName]; ok {
		if dc, ok := raw.(map[string]any); ok {
			delete(dc, capabilityName)
		}
	}

	if raw, ok := c[desiredCapabilities]; ok {
		if dc, ok := raw.(map[string]any); ok {
			delete(dc, capabilityName)
		}
	}

	if raw, ok := c[capabilities]; ok {
		if c, ok := raw.(map[string]any); ok {
			if raw, ok := c[alwaysMatch]; ok {
				if am, ok := raw.(map[string]any); ok {
					delete(am, capabilityName)
				}
			}
			if raw, ok := c[firstMatch]; ok {
				if fm, ok := raw.([]any); ok {
					for _, raw := range fm {
						if c, ok := raw.(map[string]any); ok {
							delete(c, capabilityName)
						}
					}
				}
			}
		}
	}
}

func (c Capabilities) DeepCopy() Capabilities {
	result := make(Capabilities, len(c))
	for k, v := range c {
		result[k] = deepCopyValue(v)
	}
	return result
}

func deepCopyValue(v any) any {
	switch val := v.(type) {
	case Capabilities:
		return val.DeepCopy()
	case map[string]any:
		cp := make(map[string]any, len(val))
		for kk, vv := range val {
			cp[kk] = deepCopyValue(vv)
		}
		return cp
	case []any:
		cp := make([]any, len(val))
		for i, vv := range val {
			cp[i] = deepCopyValue(vv)
		}
		return cp
	default:
		return val
	}
}
