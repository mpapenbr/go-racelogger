//nolint:lll,funlen,gocognit,gocritic,nestif,dupl // better readability
package processor

import (
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"
)

type TestData struct {
	Ref        int // used to identify records for modification
	CarClassID int
	CarID      int
	Laptiming  CarLaptiming
}

var refData = []TestData{
	{
		1,
		10,
		100,
		CarLaptiming{lap: &SectionTiming{duration: TimeWithMarker{time: 10.0, marker: ""}}},
	},
	{
		1,
		10,
		200,
		CarLaptiming{lap: &SectionTiming{duration: TimeWithMarker{time: 20.0, marker: ""}}},
	},
}

func collectCarLaptiming(d []TestData) CollectCarLaptiming {
	return func(carClassId, carId int) []*CarLaptiming {
		// copy slice refData
		work := make([]TestData, 0)
		for i, v := range d {
			if carId != -1 {
				if carId == v.CarID {
					work = append(work, d[i])
				}
			} else if carClassId != -1 {
				if carClassId == v.CarClassID {
					work = append(work, d[i])
				}
			} else {
				work = append(work, d[i])
			}
		}
		ret := make([]*CarLaptiming, len(work))
		for i := range work {
			ret[i] = &work[i].Laptiming
		}
		return ret
	}
}

func emptyBestSectionProc() *BestSectionProc {
	return NewBestSectionProc(3, []int{10, 20}, []int{100, 200}, collectCarLaptiming(refData))
}

func sampleBestSectionProc() *BestSectionProc {
	ret := NewBestSectionProc(
		3,
		[]int{10, 20},
		[]int{100, 101, 200, 201},
		collectCarLaptiming(refData),
	)
	ret.lap = map[string]float64{
		"overall": 10.0,
		"class10": 10.0, "car100": 10.0, "car101": 15.0,
		"class20": 20.0, "car200": 20.0, "car201": 25.0,
	}
	return ret
}

type STF func(*SectionTiming)

var withTime = func(t float64) STF {
	return func(st *SectionTiming) { st.duration.time = t }
}

var withPersonal = func(t float64) STF {
	return func(st *SectionTiming) { st.personalBest = t }
}

func TestBestSectionProc_markLap(t *testing.T) {
	type args struct {
		carClassID int
		carID      int
		st         []STF
	}
	tests := []struct {
		name string
		b    *BestSectionProc
		args args
		want string
	}{
		// TODO: Add test cases.
		{"Overall", emptyBestSectionProc(), args{10, 100, []STF{withTime(1.0)}}, MarkerOverallBest},
		{"Class", sampleBestSectionProc(), args{20, 200, []STF{withTime(15.0)}}, MarkerClassBest},
		{"Car", sampleBestSectionProc(), args{20, 201, []STF{withTime(23.0)}}, MarkerCarBest},
		{
			"Personal",
			sampleBestSectionProc(),
			args{10, 100, []STF{withTime(27.0), withPersonal(48)}},
			MarkerPersonalBest,
		},
		{
			"Nothing",
			sampleBestSectionProc(),
			args{10, 100, []STF{withTime(27.0), withPersonal(26)}},
			"",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prep := SectionTiming{duration: TimeWithMarker{time: -1}}
			for _, v := range tt.args.st {
				v(&prep)
			}
			got := tt.b.markLap(&prep, tt.args.carClassID, tt.args.carID)
			if got != tt.want {
				t.Errorf("TestMarkLap() = %v, want %v", got, tt.want)
			}
		})
	}
}

func patchTime(d []TestData, ref int, time float64) *TestData {
	for i, v := range d {
		if v.Ref == ref {
			d[i].Laptiming.lap.duration.time = time
			return &d[i]
		}
	}
	return nil
}

func TestBestSectionProc_markNewOB(t *testing.T) {
	tests := []struct {
		name     string
		input    []TestData
		expected []TestData
	}{
		{
			name: "Existing overall best",
			input: []TestData{
				{
					1,
					10,
					100,
					CarLaptiming{
						lap: &SectionTiming{
							duration: TimeWithMarker{time: 10.0, marker: MarkerOverallBest},
						},
					},
				},
				{
					2,
					10,
					101,
					CarLaptiming{
						lap: &SectionTiming{duration: TimeWithMarker{time: 20.0, marker: ""}},
					},
				},
			},
			expected: []TestData{
				{
					1,
					10,
					100,
					CarLaptiming{
						lap: &SectionTiming{
							duration: TimeWithMarker{time: 10.0, marker: MarkerCarBest},
						},
					},
				},
				{
					2,
					10,
					101,
					CarLaptiming{
						lap: &SectionTiming{
							duration:     TimeWithMarker{time: 5.0, marker: MarkerOverallBest},
							personalBest: 5.0,
						},
					},
				},
			},
		},
		{
			name: "Existing class best",
			input: []TestData{
				{
					1,
					10,
					100,
					CarLaptiming{
						lap: &SectionTiming{
							duration: TimeWithMarker{time: 10.0, marker: MarkerClassBest},
						},
					},
				},
				{
					2,
					10,
					101,
					CarLaptiming{
						lap: &SectionTiming{duration: TimeWithMarker{time: 20.0, marker: ""}},
					},
				},
			},
			expected: []TestData{
				{
					1,
					10,
					100,
					CarLaptiming{
						lap: &SectionTiming{
							duration: TimeWithMarker{time: 10.0, marker: MarkerCarBest},
						},
					},
				},
				{
					2,
					10,
					101,
					CarLaptiming{
						lap: &SectionTiming{
							duration:     TimeWithMarker{time: 5.0, marker: MarkerOverallBest},
							personalBest: 5.0,
						},
					},
				},
			},
		},
		{
			name: "Existing car best",
			input: []TestData{
				{
					1,
					10,
					100,
					CarLaptiming{
						lap: &SectionTiming{
							duration: TimeWithMarker{time: 10.0, marker: MarkerCarBest},
						},
					},
				},
				{
					2,
					10,
					100,
					CarLaptiming{
						lap: &SectionTiming{duration: TimeWithMarker{time: 20.0, marker: ""}},
					},
				},
			},
			expected: []TestData{
				{
					1,
					10,
					100,
					CarLaptiming{
						lap: &SectionTiming{
							duration: TimeWithMarker{time: 10.0, marker: MarkerPersonalBest},
						},
					},
				},
				{
					2,
					10,
					100,
					CarLaptiming{
						lap: &SectionTiming{
							duration:     TimeWithMarker{time: 5.0, marker: MarkerOverallBest},
							personalBest: 5.0,
						},
					},
				},
			},
		},
		{
			name: "Existing class best and car best",
			input: []TestData{
				{
					1,
					10,
					100,
					CarLaptiming{
						lap: &SectionTiming{
							duration: TimeWithMarker{time: 10.0, marker: MarkerClassBest},
						},
					},
				},
				{
					2,
					10,
					101,
					CarLaptiming{
						lap: &SectionTiming{duration: TimeWithMarker{time: 20.0, marker: ""}},
					},
				},
				{
					3,
					10,
					101,
					CarLaptiming{
						lap: &SectionTiming{
							duration: TimeWithMarker{time: 13.0, marker: MarkerCarBest},
						},
					},
				},
			},
			expected: []TestData{
				{
					1,
					10,
					100,
					CarLaptiming{
						lap: &SectionTiming{
							duration: TimeWithMarker{time: 10.0, marker: MarkerCarBest},
						},
					},
				},
				{
					2,
					10,
					101,
					CarLaptiming{
						lap: &SectionTiming{
							duration:     TimeWithMarker{time: 5.0, marker: MarkerOverallBest},
							personalBest: 5.0,
						},
					},
				},
				{
					3,
					10,
					101,
					CarLaptiming{
						lap: &SectionTiming{
							duration: TimeWithMarker{time: 13.0, marker: MarkerPersonalBest},
						},
					},
				},
			},
		},

		{
			name: "Existing class best and car best degrading #1",
			input: []TestData{
				{
					1,
					10,
					100,
					CarLaptiming{
						lap: &SectionTiming{
							duration: TimeWithMarker{time: 10.0, marker: MarkerClassBest},
						},
					},
				},
				{
					2,
					10,
					100,
					CarLaptiming{
						lap: &SectionTiming{duration: TimeWithMarker{time: 20.0, marker: ""}},
					},
				},
				{
					3,
					10,
					101,
					CarLaptiming{
						lap: &SectionTiming{
							duration: TimeWithMarker{time: 13.0, marker: MarkerCarBest},
						},
					},
				},
			},
			expected: []TestData{
				{
					1,
					10,
					100,
					CarLaptiming{
						lap: &SectionTiming{
							duration: TimeWithMarker{time: 10.0, marker: MarkerPersonalBest},
						},
					},
				},
				{
					2,
					10,
					100,
					CarLaptiming{
						lap: &SectionTiming{
							duration:     TimeWithMarker{time: 5.0, marker: MarkerOverallBest},
							personalBest: 5.0,
						},
					},
				},
				{
					3,
					10,
					101,
					CarLaptiming{
						lap: &SectionTiming{
							duration: TimeWithMarker{time: 13.0, marker: MarkerCarBest},
						},
					},
				},
			},
		},

		{
			name: "Existing class best and car best II",
			input: []TestData{
				{
					1,
					10,
					100,
					CarLaptiming{
						lap: &SectionTiming{
							duration: TimeWithMarker{time: 10.0, marker: MarkerClassBest},
						},
					},
				},
				{
					2,
					10,
					101,
					CarLaptiming{
						lap: &SectionTiming{duration: TimeWithMarker{time: 20.0, marker: ""}},
					},
				},
				{
					3,
					10,
					101,
					CarLaptiming{
						lap: &SectionTiming{
							duration: TimeWithMarker{time: 13.0, marker: MarkerCarBest},
						},
					},
				},
				{
					4,
					20,
					200,
					CarLaptiming{
						lap: &SectionTiming{
							duration: TimeWithMarker{time: 11.0, marker: MarkerClassBest},
						},
					},
				},
			},
			expected: []TestData{
				{
					1,
					10,
					100,
					CarLaptiming{
						lap: &SectionTiming{
							duration: TimeWithMarker{time: 10.0, marker: MarkerCarBest},
						},
					},
				},
				{
					2,
					10,
					101,
					CarLaptiming{
						lap: &SectionTiming{
							duration:     TimeWithMarker{time: 5.0, marker: MarkerOverallBest},
							personalBest: 5.0,
						},
					},
				},
				{
					3,
					10,
					101,
					CarLaptiming{
						lap: &SectionTiming{
							duration: TimeWithMarker{time: 13.0, marker: MarkerPersonalBest},
						},
					},
				},
				{
					4,
					20,
					200,
					CarLaptiming{
						lap: &SectionTiming{
							duration: TimeWithMarker{time: 11.0, marker: MarkerClassBest},
						},
					},
				},
			},
		},

		{
			name: "Existing class best and car best III",
			input: []TestData{
				{
					1,
					10,
					100,
					CarLaptiming{
						lap: &SectionTiming{
							duration: TimeWithMarker{time: 10.0, marker: MarkerOverallBest},
						},
					},
				},
				{
					2,
					10,
					101,
					CarLaptiming{
						lap: &SectionTiming{duration: TimeWithMarker{time: 20.0, marker: ""}},
					},
				},
				{
					3,
					10,
					101,
					CarLaptiming{
						lap: &SectionTiming{
							duration: TimeWithMarker{time: 13.0, marker: MarkerCarBest},
						},
					},
				},
				{
					4,
					20,
					200,
					CarLaptiming{
						lap: &SectionTiming{
							duration: TimeWithMarker{time: 11.0, marker: MarkerClassBest},
						},
					},
				},
			},
			expected: []TestData{
				{
					1,
					10,
					100,
					CarLaptiming{
						lap: &SectionTiming{
							duration: TimeWithMarker{time: 10.0, marker: MarkerCarBest},
						},
					},
				},
				{
					2,
					10,
					101,
					CarLaptiming{
						lap: &SectionTiming{
							duration:     TimeWithMarker{time: 5.0, marker: MarkerOverallBest},
							personalBest: 5.0,
						},
					},
				},
				{
					3,
					10,
					101,
					CarLaptiming{
						lap: &SectionTiming{
							duration: TimeWithMarker{time: 13.0, marker: MarkerPersonalBest},
						},
					},
				},
				{
					4,
					20,
					200,
					CarLaptiming{
						lap: &SectionTiming{
							duration: TimeWithMarker{time: 11.0, marker: MarkerClassBest},
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			proc := *sampleBestSectionProc()
			proc.collectFromOther = collectCarLaptiming(tt.input)

			item := patchTime(tt.input, 2, 5.0)

			proc.markLap(item.Laptiming.lap, item.CarClassID, item.CarID)
			if reflect.DeepEqual(tt.input, tt.expected) == false {
				diff := cmp.Diff(
					tt.input,
					tt.expected,
					cmp.AllowUnexported(CarLaptiming{}, SectionTiming{}, TimeWithMarker{}),
				)
				t.Errorf(
					"TestBestSectionProc_markNewOB() = %v, want %v diff: %s",
					tt.input,
					tt.expected,
					diff,
				)
			}
		})
	}
}

func TestBestSectionProc_markClassBest(t *testing.T) {
	type args struct {
		ref  int
		time float64
	}
	tests := []struct {
		name      string
		input     []TestData
		patchArgs args
		expected  []TestData
	}{
		{
			name: "New class best",
			input: []TestData{
				{
					1,
					10,
					100,
					CarLaptiming{
						lap: &SectionTiming{
							duration: TimeWithMarker{time: 10.0, marker: MarkerOverallBest},
						},
					},
				},
				{
					2,
					20,
					200,
					CarLaptiming{
						lap: &SectionTiming{duration: TimeWithMarker{time: 20.0, marker: ""}},
					},
				},
			},
			patchArgs: args{2, 15.0},
			expected: []TestData{
				{
					1,
					10,
					100,
					CarLaptiming{
						lap: &SectionTiming{
							duration: TimeWithMarker{time: 10.0, marker: MarkerOverallBest},
						},
					},
				},
				{
					2,
					20,
					200,
					CarLaptiming{
						lap: &SectionTiming{
							duration:     TimeWithMarker{time: 15.0, marker: MarkerClassBest},
							personalBest: 15.0,
						},
					},
				},
			},
		},
		{
			name: "Degrade existing class best",
			input: []TestData{
				{
					1,
					20,
					200,
					CarLaptiming{
						lap: &SectionTiming{
							duration: TimeWithMarker{time: 20.0, marker: MarkerClassBest},
						},
					},
				},
				{
					2,
					20,
					200,
					CarLaptiming{
						lap: &SectionTiming{duration: TimeWithMarker{time: 21.0, marker: ""}},
					},
				},
			},
			patchArgs: args{2, 15.0},
			expected: []TestData{
				{
					1,
					20,
					200,
					CarLaptiming{
						lap: &SectionTiming{
							duration: TimeWithMarker{time: 20.0, marker: MarkerPersonalBest},
						},
					},
				},
				{
					2,
					20,
					200,
					CarLaptiming{
						lap: &SectionTiming{
							duration:     TimeWithMarker{time: 15.0, marker: MarkerClassBest},
							personalBest: 15.0,
						},
					},
				},
			},
		},
		{
			name: "Degrade existing class best keeping ",
			input: []TestData{
				{
					1,
					20,
					200,
					CarLaptiming{
						lap: &SectionTiming{
							duration: TimeWithMarker{time: 20.0, marker: MarkerClassBest},
						},
					},
				},
				{
					2,
					20,
					201,
					CarLaptiming{
						lap: &SectionTiming{duration: TimeWithMarker{time: 21.0, marker: ""}},
					},
				},
				{
					3,
					20,
					201,
					CarLaptiming{
						lap: &SectionTiming{
							duration: TimeWithMarker{time: 21.0, marker: MarkerCarBest},
						},
					},
				},
			},
			patchArgs: args{2, 15.0},
			expected: []TestData{
				{
					1,
					20,
					200,
					CarLaptiming{
						lap: &SectionTiming{
							duration: TimeWithMarker{time: 20.0, marker: MarkerCarBest},
						},
					},
				},
				{
					2,
					20,
					201,
					CarLaptiming{
						lap: &SectionTiming{
							duration:     TimeWithMarker{time: 15.0, marker: MarkerClassBest},
							personalBest: 15.0,
						},
					},
				},
				{
					3,
					20,
					201,
					CarLaptiming{
						lap: &SectionTiming{
							duration: TimeWithMarker{time: 21.0, marker: MarkerPersonalBest},
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			proc := sampleBestSectionProc()
			proc.collectFromOther = collectCarLaptiming(tt.input)

			item := patchTime(tt.input, tt.patchArgs.ref, tt.patchArgs.time)

			proc.markLap(item.Laptiming.lap, item.CarClassID, item.CarID)
			if reflect.DeepEqual(tt.input, tt.expected) == false {
				diff := cmp.Diff(
					tt.input,
					tt.expected,
					cmp.AllowUnexported(CarLaptiming{}, SectionTiming{}, TimeWithMarker{}),
				)
				t.Errorf(
					"TestBestSectionProc_markClassBest() = %v, want %v diff: %s",
					tt.input,
					tt.expected,
					diff,
				)
			}
		})
	}
}

func TestBestSectionProc_markCarBest(t *testing.T) {
	type args struct {
		ref  int
		time float64
	}
	tests := []struct {
		name      string
		input     []TestData
		patchArgs args
		expected  []TestData
	}{
		{
			name: "New car best",
			input: []TestData{
				{
					1,
					10,
					100,
					CarLaptiming{
						lap: &SectionTiming{
							duration: TimeWithMarker{time: 10.0, marker: MarkerOverallBest},
						},
					},
				},
				{
					2,
					20,
					201,
					CarLaptiming{
						lap: &SectionTiming{duration: TimeWithMarker{time: 30.0, marker: ""}},
					},
				},
			},
			patchArgs: args{2, 23.0},
			expected: []TestData{
				{
					1,
					10,
					100,
					CarLaptiming{
						lap: &SectionTiming{
							duration: TimeWithMarker{time: 10.0, marker: MarkerOverallBest},
						},
					},
				},
				{
					2,
					20,
					201,
					CarLaptiming{
						lap: &SectionTiming{
							duration:     TimeWithMarker{time: 23.0, marker: MarkerCarBest},
							personalBest: 23.0,
						},
					},
				},
			},
		},
		{
			name: "Degrade existing car best",
			input: []TestData{
				{
					1,
					20,
					201,
					CarLaptiming{
						lap: &SectionTiming{
							duration: TimeWithMarker{time: 24.0, marker: MarkerCarBest},
						},
					},
				},
				{
					2,
					20,
					201,
					CarLaptiming{
						lap: &SectionTiming{duration: TimeWithMarker{time: 30.0, marker: ""}},
					},
				},
			},
			patchArgs: args{2, 23.0},
			expected: []TestData{
				{
					1,
					20,
					201,
					CarLaptiming{
						lap: &SectionTiming{
							duration: TimeWithMarker{time: 24.0, marker: MarkerPersonalBest},
						},
					},
				},
				{
					2,
					20,
					201,
					CarLaptiming{
						lap: &SectionTiming{
							duration:     TimeWithMarker{time: 23.0, marker: MarkerCarBest},
							personalBest: 23.0,
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			proc := sampleBestSectionProc()
			proc.collectFromOther = collectCarLaptiming(tt.input)

			item := patchTime(tt.input, tt.patchArgs.ref, tt.patchArgs.time)

			proc.markLap(item.Laptiming.lap, item.CarClassID, item.CarID)
			if reflect.DeepEqual(tt.input, tt.expected) == false {
				diff := cmp.Diff(
					tt.input,
					tt.expected,
					cmp.AllowUnexported(CarLaptiming{}, SectionTiming{}, TimeWithMarker{}),
				)
				t.Errorf(
					"TestBestSectionProc_markCarBest() = %v, want %v diff: %s",
					tt.input,
					tt.expected,
					diff,
				)
			}
		})
	}
}

func TestBestSectionProc_removeMarker(t *testing.T) {
	type args struct {
		ref  int
		time float64
	}
	tests := []struct {
		name      string
		input     []TestData
		patchArgs args
		expected  []TestData
	}{
		{
			name: "Existing overall best",
			input: []TestData{
				{
					1,
					10,
					100,
					CarLaptiming{
						lap: &SectionTiming{duration: TimeWithMarker{time: 30.0, marker: ""}},
					},
				},
				{
					2,
					10,
					101,
					CarLaptiming{
						lap: &SectionTiming{
							duration:     TimeWithMarker{time: 20.0, marker: MarkerOverallBest},
							personalBest: 20.0,
						},
					},
				},
			},
			patchArgs: args{2, 25.0},
			expected: []TestData{
				{
					1,
					10,
					100,
					CarLaptiming{
						lap: &SectionTiming{duration: TimeWithMarker{time: 30.0, marker: ""}},
					},
				},
				{
					2,
					10,
					101,
					CarLaptiming{
						lap: &SectionTiming{
							duration:     TimeWithMarker{time: 25.0, marker: ""},
							personalBest: 20.0,
						},
					},
				},
			},
		},
		{
			name: "Existing class best",
			input: []TestData{
				{
					1,
					10,
					100,
					CarLaptiming{
						lap: &SectionTiming{duration: TimeWithMarker{time: 30.0, marker: ""}},
					},
				},
				{
					2,
					10,
					101,
					CarLaptiming{
						lap: &SectionTiming{
							duration:     TimeWithMarker{time: 20.0, marker: MarkerClassBest},
							personalBest: 20.0,
						},
					},
				},
			},
			patchArgs: args{2, 25.0},
			expected: []TestData{
				{
					1,
					10,
					100,
					CarLaptiming{
						lap: &SectionTiming{duration: TimeWithMarker{time: 30.0, marker: ""}},
					},
				},
				{
					2,
					10,
					101,
					CarLaptiming{
						lap: &SectionTiming{
							duration:     TimeWithMarker{time: 25.0, marker: ""},
							personalBest: 20.0,
						},
					},
				},
			},
		},
		{
			name: "Existing car best",
			input: []TestData{
				{
					1,
					10,
					100,
					CarLaptiming{
						lap: &SectionTiming{duration: TimeWithMarker{time: 30.0, marker: ""}},
					},
				},
				{
					2,
					10,
					101,
					CarLaptiming{
						lap: &SectionTiming{
							duration:     TimeWithMarker{time: 20.0, marker: MarkerCarBest},
							personalBest: 20.0,
						},
					},
				},
			},
			patchArgs: args{2, 25.0},
			expected: []TestData{
				{
					1,
					10,
					100,
					CarLaptiming{
						lap: &SectionTiming{duration: TimeWithMarker{time: 30.0, marker: ""}},
					},
				},
				{
					2,
					10,
					101,
					CarLaptiming{
						lap: &SectionTiming{
							duration:     TimeWithMarker{time: 25.0, marker: ""},
							personalBest: 20.0,
						},
					},
				},
			},
		},
		{
			name: "Existing personal best",
			input: []TestData{
				{
					1,
					10,
					100,
					CarLaptiming{
						lap: &SectionTiming{duration: TimeWithMarker{time: 30.0, marker: ""}},
					},
				},
				{
					2,
					10,
					101,
					CarLaptiming{
						lap: &SectionTiming{
							duration:     TimeWithMarker{time: 20.0, marker: MarkerPersonalBest},
							personalBest: 20.0,
						},
					},
				},
			},
			patchArgs: args{2, 25.0},
			expected: []TestData{
				{
					1,
					10,
					100,
					CarLaptiming{
						lap: &SectionTiming{duration: TimeWithMarker{time: 30.0, marker: ""}},
					},
				},
				{
					2,
					10,
					101,
					CarLaptiming{
						lap: &SectionTiming{
							duration:     TimeWithMarker{time: 25.0, marker: ""},
							personalBest: 20.0,
						},
					},
				},
			},
		},
		{
			name: "Keep marker if not slower",
			input: []TestData{
				{
					1,
					10,
					100,
					CarLaptiming{
						lap: &SectionTiming{duration: TimeWithMarker{time: 30.0, marker: ""}},
					},
				},
				{
					2,
					10,
					101,
					CarLaptiming{
						lap: &SectionTiming{
							duration:     TimeWithMarker{time: 20.0, marker: MarkerPersonalBest},
							personalBest: 20.0,
						},
					},
				},
			},
			patchArgs: args{2, 20.0},
			expected: []TestData{
				{
					1,
					10,
					100,
					CarLaptiming{
						lap: &SectionTiming{duration: TimeWithMarker{time: 30.0, marker: ""}},
					},
				},
				{
					2,
					10,
					101,
					CarLaptiming{
						lap: &SectionTiming{
							duration:     TimeWithMarker{time: 20.0, marker: MarkerPersonalBest},
							personalBest: 20.0,
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			proc := sampleBestSectionProc()
			proc.collectFromOther = collectCarLaptiming(tt.input)

			item := patchTime(tt.input, tt.patchArgs.ref, tt.patchArgs.time)

			proc.markLap(item.Laptiming.lap, item.CarClassID, item.CarID)
			if reflect.DeepEqual(tt.input, tt.expected) == false {
				diff := cmp.Diff(
					tt.input,
					tt.expected,
					cmp.AllowUnexported(CarLaptiming{}, SectionTiming{}, TimeWithMarker{}),
				)
				t.Errorf(
					"TestBestSectionProc_markNewOB() = %v, want %v diff: %s",
					tt.input,
					tt.expected,
					diff,
				)
			}
		})
	}
}
