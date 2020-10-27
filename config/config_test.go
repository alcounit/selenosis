package config

import (
	"errors"
	"io/ioutil"
	"log"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConfigFileData(t *testing.T) {

	tests := map[string]struct {
		data   string
		config string
		err    error
	}{
		"verify empty JSON config is not allowed": {
			data:   ``,
			config: "browsers.json",
			err:    errors.New("parse error: EOF"),
		},
		"verify empty YAML config is not allowed": {
			data:   ``,
			config: "browsers.yaml",
			err:    errors.New("parse error: EOF"),
		},
		"verify invalid characters not allowed for JSON config": {
			data: `{
				"chrome": {
				  "path": "/",
				  "spec": {
					"resources": {
					  "requests": {
						"memory": "500Mi",
						"cpu": "0.5"`,
			config: "browsers.json",
			err:    errors.New("parse error: unexpected EOF"),
		},
		"verify invalid characters not allowed for YAML config": {
			data: `---
			chrome:
			  spec:
				resources:
				  cpu: 500m
				  memory: 1Gi
				hostAliases:`,
			config: "browsers.yaml",
			err:    errors.New("parse error: error converting YAML to JSON: yaml: line 2: found character that cannot start any token"),
		},
		"verify empty JSON config is allowed ": {
			data:   `{}`,
			config: "browsers.json",
			err:    errors.New("empty config: <nil>"),
		},
		"verify empty YAML config is allowed ": {
			data:   `---`,
			config: "browsers.yaml",
			err:    errors.New("empty config: <nil>"),
		},
	}

	for name, test := range tests {
		t.Logf("TC: %s", name)
		f := configfile(test.data, test.config)
		defer os.Remove(f)
		_, err := NewBrowsersConfig(f)
		assert.Equal(t, test.err, err)
	}
}

func TestConfigFile(t *testing.T) {

	var empty string

	tests := map[string]struct {
		data string
		err  error
	}{
		"verify config file not exist": {
			data: empty,
			err:  errors.New("read error: open : The system cannot find the file specified."),
		},
	}

	for name, test := range tests {
		t.Logf("TC: %s", name)
		_, err := NewBrowsersConfig(test.data)
		assert.Equal(t, test.err, err)
	}
}

func TestConfig(t *testing.T) {

	tests := map[string]struct {
		data string
		err  error
	}{
		"verify yaml config file": {
			data: "browsers.yaml",
			err:  nil,
		},
		"verify json config file": {
			data: "browsers.json",
			err:  nil,
		},
	}

	for name, test := range tests {
		t.Logf("TC: %s", name)
		_, err := NewBrowsersConfig(test.data)
		assert.Equal(t, test.err, err)
	}
}

func TestConfigSearch(t *testing.T) {
	tests := map[string]struct {
		browserName    string
		browserVersion string
		config         string
		err            error
	}{
		"verify empty browser name input for JSON config file": {
			browserVersion: "68.0",
			config:         "browsers.json",
			err:            errors.New("unknown browser name "),
		},
		"verify empty browser name input for YAML config file": {
			browserVersion: "68.0",
			config:         "browsers.yaml",
			err:            errors.New("unknown browser name "),
		},
		"verify empty browser version for JSON config file": {
			browserName: "chrome",
			config:      "browsers.json",
			err:         errors.New("unknown browser version "),
		},
		"verify empty browser version for YAML config file": {
			browserName: "chrome",
			config:      "browsers.yaml",
			err:         errors.New("unknown browser version "),
		},
		"verify non existing browser name for JSON config file": {
			browserName: "amigo",
			config:      "browsers.json",
			err:         errors.New("unknown browser name amigo"),
		},
		"verify non existing browser name for YAML config file": {
			browserName: "amigo",
			config:      "browsers.yaml",
			err:         errors.New("unknown browser name amigo"),
		},
		"verify non existing browser version for JSON config file": {
			browserName:    "chrome",
			browserVersion: "0.1",
			config:         "browsers.json",
			err:            errors.New("unknown browser version 0.1"),
		},
		"verify non existing browser version for YAML config file": {
			browserName:    "chrome",
			browserVersion: "0.1",
			config:         "browsers.yaml",
			err:            errors.New("unknown browser version 0.1"),
		},
		"verify no error if correct data provided for JSON config file": {
			browserName:    "chrome",
			browserVersion: "68.0",
			config:         "browsers.json",
			err:            nil,
		},
		"verify no error if correct data provided for YAML config file": {
			browserName:    "chrome",
			browserVersion: "68.0",
			config:         "browsers.yaml",
			err:            nil,
		},
	}

	for name, test := range tests {
		t.Logf("TC: %s", name)
		c, err := NewBrowsersConfig(test.config)
		if err != nil {
			t.Errorf("error loading config %v", err)
		}
		_, err = c.Find(test.browserName, test.browserVersion)
		assert.Equal(t, test.err, err)
	}

}

func configfile(data string, config string) string {
	tmp, err := ioutil.TempFile("", config)
	if err != nil {
		log.Fatal(err)
	}
	_, err = tmp.Write([]byte(data))
	if err != nil {
		log.Fatal(err)
	}
	err = tmp.Close()
	if err != nil {
		log.Fatal(err)
	}
	return tmp.Name()
}