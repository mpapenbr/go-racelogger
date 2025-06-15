//nolint:funlen // by design for tests
package recorder

import (
	"testing"
)

func Test_computeNameAndDescription(t *testing.T) {
	type args struct {
		cliNames []string
		cliDescr []string
		idx      int
	}
	tests := []struct {
		name            string
		args            args
		wantName        string
		wantDescription string
	}{
		{
			name: "no names or descriptions",
			args: args{
				cliNames: []string{},
				cliDescr: []string{},
				idx:      0,
			},
			wantName:        "",
			wantDescription: "",
		},
		{
			name: "no names or descriptions (index out of range)",
			args: args{
				cliNames: []string{},
				cliDescr: []string{},
				idx:      1,
			},
			wantName:        "",
			wantDescription: "",
		},
		{
			name: "name without description",
			args: args{
				cliNames: []string{"test_name"},
				cliDescr: []string{},
				idx:      0,
			},
			wantName:        "test_name",
			wantDescription: "",
		},
		{
			name: "name and description",
			args: args{
				cliNames: []string{"test_name"},
				cliDescr: []string{"test_description"},
				idx:      0,
			},
			wantName:        "test_name",
			wantDescription: "test_description",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotName, gotDescription := computeNameAndDescription(
				tt.args.cliNames,
				tt.args.cliDescr,
				tt.args.idx,
			)
			if gotName != tt.wantName {
				t.Errorf("computeNameAndDescription() gotName = %v, want %v", gotName, tt.wantName)
			}
			if gotDescription != tt.wantDescription {
				t.Errorf(
					"computeNameAndDescription() gotDescription = %v, want %v",
					gotDescription,
					tt.wantDescription,
				)
			}
		})
	}
}

func Test_simulateParamsForHeatRaces(t *testing.T) {
	type args struct {
		cliNames []string
		cliDescr []string
		idx      int
	}

	tests := []struct {
		name            string
		args            args
		wantName        string
		wantDescription string
	}{
		{
			name: "matching first index",
			args: args{
				cliNames: []string{"test_name1", "test_name2"},
				cliDescr: []string{"test_description1", "test_description2"},
				idx:      0,
			},
			wantName:        "test_name1",
			wantDescription: "test_description1",
		},

		{
			name: "matching second index",
			args: args{
				cliNames: []string{"test_name1", "test_name2"},
				cliDescr: []string{"test_description1", "test_description2"},
				idx:      1,
			},
			wantName:        "test_name2",
			wantDescription: "test_description2",
		},
		{
			name: "index out of range - use last element",
			args: args{
				cliNames: []string{"test_name1", "test_name2"},
				cliDescr: []string{"test_description1", "test_description2"},
				idx:      2,
			},
			wantName:        "test_name2",
			wantDescription: "test_description2",
		},
		{
			name: "mix 1",
			args: args{
				cliNames: []string{"test_name"},
				cliDescr: []string{"test_description1", "test_description2"},
				idx:      0,
			},
			wantName:        "test_name",
			wantDescription: "test_description1",
		},
		{
			name: "mix 2 (out of range name)",
			args: args{
				cliNames: []string{"test_name"},
				cliDescr: []string{"test_description1", "test_description2"},
				idx:      1,
			},
			wantName:        "test_name",
			wantDescription: "test_description2",
		},
		{
			name: "mix 3 (out of range both)",
			args: args{
				cliNames: []string{"test_name"},
				cliDescr: []string{"test_description1", "test_description2"},
				idx:      2,
			},
			wantName:        "test_name",
			wantDescription: "test_description2",
		},
		{
			name: "mix 4 (out of range description)",
			args: args{
				cliNames: []string{"test_name1", "test_name2"},
				cliDescr: []string{"test_description"},
				idx:      1,
			},
			wantName:        "test_name2",
			wantDescription: "test_description",
		},
		{
			name: "mix 5 (no names)",
			args: args{
				cliNames: []string{},
				cliDescr: []string{"test_description1", "test_description2"},
				idx:      1,
			},
			wantName:        "",
			wantDescription: "test_description2",
		},
		{
			name: "mix 6 (no descriptions)",
			args: args{
				cliNames: []string{"test_name1", "test_name2"},
				cliDescr: []string{},
				idx:      1,
			},
			wantName:        "test_name2",
			wantDescription: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotName, gotDescription := computeNameAndDescription(
				tt.args.cliNames,
				tt.args.cliDescr,
				tt.args.idx,
			)
			if gotName != tt.wantName {
				t.Errorf("simulateHeatRaces() gotName = %v, want %v", gotName, tt.wantName)
			}
			if gotDescription != tt.wantDescription {
				t.Errorf(
					"simulateHeatRaces() gotDescription = %v, want %v",
					gotDescription,
					tt.wantDescription,
				)
			}
		})
	}
}
