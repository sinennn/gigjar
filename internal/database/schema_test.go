package database

import "testing"

func TestSchemaStatusCheck(t *testing.T) {
	cases := []struct {
		name    string
		status  SchemaStatus
		wantErr bool
	}{
		{"current", SchemaStatus{Version: 3}, false},
		{"behind", SchemaStatus{Version: 2}, true},
		{"ahead", SchemaStatus{Version: 4}, true},
		{"dirty", SchemaStatus{Version: 3, Dirty: true}, true},
		{"empty", SchemaStatus{}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.status.Check(3)
			if (err != nil) != tc.wantErr {
				t.Errorf("Check() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}
