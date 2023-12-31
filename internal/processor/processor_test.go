//nolint:funlen // by design for tests
package processor

import (
	"testing"

	iryaml "github.com/mpapenbr/goirsdk/yaml"
	"gopkg.in/yaml.v3"
)

func TestProcessor_hasDriverChange(t *testing.T) {
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
	p := &Processor{}
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
			if got := p.hasDriverChange(&current, &last); got != tt.want {
				t.Errorf("Processor.hasDriverChange() = %v, want %v", got, tt.want)
			}
		})
	}
}
