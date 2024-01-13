//nolint:funlen,lll // by design for tests
package processor

import (
	"testing"

	"github.com/mpapenbr/goirsdk/irsdk"
	iryaml "github.com/mpapenbr/goirsdk/yaml"
	"gopkg.in/yaml.v3"
)

func TestGetMetricUnit(t *testing.T) {
	type args struct {
		s string
	}
	tests := []struct {
		name    string
		args    args
		want    float64
		wantErr bool
	}{
		{"kph", args{"1 kph"}, 1.0, false},
		{"percent", args{"2 %"}, 2.0, false},
		{"celsius", args{"3 °C"}, 3.0, false},
		{"without unit", args{"4"}, 4.0, false},
		{"miles", args{"4 mi"}, 4 * 1.60934, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetMetricUnit(tt.args.s)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetMetricUnit() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("GetMetricUnit() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHasDriverChange(t *testing.T) {
	type args struct {
		current string
		last    string
	}

	tests := []struct {
		name string
		args args
		want bool
	}{
		// TODO: Add test cases.
		{
			"no change",
			args{
				current: `
Drivers:
- CarIdx: 1
  UserName: A
`,
				last: `
Drivers:
- CarIdx: 1
  UserName: A
`,
			},
			false,
		},
		{
			"driver name change",
			args{
				current: `
Drivers:
- CarIdx: 1
  UserName: A
`,
				last: `
Drivers:
- CarIdx: 1
  UserName: B
`,
			},
			true,
		},
		{
			"same size, additional driver",
			args{
				current: `
Drivers:
- CarIdx: 1
  UserName: A
`,
				last: `
Drivers:
- CarIdx: 2
  UserName: B
`,
			},
			true,
		},
		{
			"additional entry",
			args{
				current: `
Drivers:
- CarIdx: 1
  UserName: A
- CarIdx: 2
  UserName: B  
`,
				last: `
Drivers:
- CarIdx: 1
  UserName: A

`,
			},
			true,
		},
	}

	for _, tt := range tests {
		var current, last iryaml.DriverInfo
		var err error
		t.Run(tt.name, func(t *testing.T) {
			err = yaml.Unmarshal([]byte(tt.args.current), &current)
			if err != nil {
				t.Errorf("Error unmarshalling current yaml: %v", err)
			}
			err = yaml.Unmarshal([]byte(tt.args.last), &last)
			if err != nil {
				t.Errorf("Error unmarshalling last yaml: %v", err)
			}
			if got := HasDriverChange(&current, &last); got != tt.want {
				t.Errorf("HasDriverChange() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_computeFlagState(t *testing.T) {
	type args struct {
		state int32
		flags int64
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{"prep", args{int32(irsdk.StateGetInCar), 0}, "PREP"},
		{"parade", args{int32(irsdk.StateParadeLaps), 0}, "PARADE"},
		{"parade (1 to green)", args{int32(irsdk.StateParadeLaps), 0x80000604}, "PARADE"},
		{"green (switch state,keep flags)", args{int32(irsdk.StateRacing), 0x80000604}, "GREEN"},
		{"green (state + green flag)", args{int32(irsdk.StateRacing), 0x80000004}, "GREEN"},
		{"green (no extra flag)", args{int32(irsdk.StateRacing), int64(0x10000000)}, "GREEN"},
		{"green (green flag)", args{int32(irsdk.StateRacing), int64(0x10000004)}, "GREEN"},
		{"yellow (caution waving)", args{int32(irsdk.StateRacing), int64(0x10008000)}, "YELLOW"},
		{"yellow (caution)", args{int32(irsdk.StateRacing), int64(0x10004000)}, "YELLOW"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := computeFlagState(tt.args.state, tt.args.flags); got != tt.want {
				t.Errorf("computeFlagState() = %v, want %v", got, tt.want)
			}
		})
	}
}
