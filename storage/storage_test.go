package storage

import (
	"testing"

	"github.com/alcounit/selenosis/platform"
	"gotest.tools/assert"
)

func TestNew(t *testing.T) {
	tests := map[string]struct {
		len int
	}{
		"Verify storage is empty on creation": {
			len: 0,
		},
	}

	for name, test := range tests {
		t.Logf("TC: %s", name)

		strg := New()

		assert.Equal(t, strg.Sessions().Len(), test.len)
		assert.Equal(t, strg.Workers().Len(), test.len)
	}
}

func TestPut(t *testing.T) {
	tests := map[string]struct {
		strg    *Storage
		session string
		service platform.Service
		len     int
	}{
		"Verify service put to storage": {
			strg:    New(),
			session: "selenoid-vnc-chrome-85-0-c3fa5fa2-ea17-4b16-adec-97f7d535ee93",
			service: platform.Service{},
			len:     1,
		},
		"Verify service put to storage on empty session": {
			strg:    New(),
			session: "",
			service: platform.Service{},
			len:     0,
		},
	}

	for name, test := range tests {
		t.Logf("TC: %s", name)

		test.strg.Sessions().Put(test.session, test.service)

		assert.Equal(t, test.strg.Sessions().Len(), test.len)
	}
}

func TestDelete(t *testing.T) {
	tests := map[string]struct {
		strg            *Storage
		sessionToAdd    string
		sessionToDelete string
		service         platform.Service
		lenOnAdd        int
		lenOnDelete     int
	}{
		"Verify storage size when existing session deleted": {
			strg:            New(),
			sessionToAdd:    "selenoid-vnc-chrome-85-0-c3fa5fa2-ea17-4b16-adec-97f7d535ee93",
			sessionToDelete: "selenoid-vnc-chrome-85-0-c3fa5fa2-ea17-4b16-adec-97f7d535ee93",
			service:         platform.Service{},
			lenOnAdd:        1,
			lenOnDelete:     0,
		},
		"Verify storage size when non existing session deleted": {
			strg:            New(),
			sessionToAdd:    "selenoid-vnc-chrome-85-0-c3fa5fa2-ea17-4b16-adec-97f7d535ee93",
			sessionToDelete: "selenoid-vnc-chrome-85-0-c3fa5fa2-ea17-4b16-adec-97f7d535ee92",
			service:         platform.Service{},
			lenOnAdd:        1,
			lenOnDelete:     1,
		},
	}

	for name, test := range tests {
		t.Logf("TC: %s", name)

		test.strg.Sessions().Put(test.sessionToAdd, test.service)

		assert.Equal(t, test.strg.Sessions().Len(), test.lenOnAdd)

		test.strg.Sessions().Delete(test.sessionToDelete)

		assert.Equal(t, test.strg.Sessions().Len(), test.lenOnDelete)
	}
}

func TestList(t *testing.T) {
	tests := map[string]struct {
		strg    *Storage
		session string
		service platform.Service
		len     int
	}{
		"Verify storage listing": {
			strg:    New(),
			session: "selenoid-vnc-chrome-85-0-c3fa5fa2-ea17-4b16-adec-97f7d535ee93",
			service: platform.Service{
				SessionID: "selenoid-vnc-chrome-85-0-c3fa5fa2-ea17-4b16-adec-97f7d535ee93",
			},
			len: 1,
		},
	}

	for name, test := range tests {
		t.Logf("TC: %s", name)

		test.strg.Sessions().Put(test.session, test.service)

		for _, svc := range test.strg.Sessions().List() {
			assert.Equal(t, svc.SessionID, test.service.SessionID)
		}

		assert.Equal(t, test.strg.Sessions().Len(), test.len)
	}
}
