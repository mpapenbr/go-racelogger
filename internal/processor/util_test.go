package processor

import "testing"

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
