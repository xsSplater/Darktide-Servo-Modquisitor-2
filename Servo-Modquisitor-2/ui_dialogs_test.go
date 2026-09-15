// Servo-Modquisitor-2/ui_dialogs_test.go
package main

import "testing"

func TestFileInfoSnapshot(t *testing.T) {
	cases := []struct {
		name        string
		in          *FileInfo
		wantVersion string
		wantTS      int64
		wantName    string
	}{
		{
			name:        "nil fileInfo",
			in:          nil,
			wantVersion: "",
			wantTS:      0,
			wantName:    "",
		},
		{
			name:        "empty fileInfo",
			in:          &FileInfo{},
			wantVersion: "",
			wantTS:      0,
			wantName:    "",
		},
		{
			name:        "filled fileInfo",
			in:          &FileInfo{Version: "1.2.3", UploadedTimestamp: 1700000000, FileName: "mod.zip"},
			wantVersion: "1.2.3",
			wantTS:      1700000000,
			wantName:    "mod.zip",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v, ts, n := fileInfoSnapshot(tc.in)
			if v != tc.wantVersion || ts != tc.wantTS || n != tc.wantName {
				t.Errorf("snapshot(%+v) = (%q, %d, %q), want (%q, %d, %q)",
					tc.in, v, ts, n, tc.wantVersion, tc.wantTS, tc.wantName)
			}
		})
	}
}
