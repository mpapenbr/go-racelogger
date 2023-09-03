package processor

import (
	"math"
	"testing"

	"github.com/mpapenbr/iracelog-service-manager-go/pkg/model"
)

func defaultTestSpeedmapProc() *SpeedmapProc {
	ret := NewSpeedmapProc(
		nil, // don't need api for testing
		10,
		&GlobalProcessingData{TrackInfo: model.TrackInfo{Length: 100}})
	createChunks := func(avgs []float64) []*ChunkData {
		chunks := make([]*ChunkData, len(avgs))
		for i, v := range avgs {
			chunks[i] = &ChunkData{id: i, avg: v}
		}
		return chunks
	}
	ret.carClassLookup[1] = createChunks([]float64{100, 100, 100, 100, 100, 100, 100, 100, 100, 100})
	return ret
}

func almostEqual(a, b float64) bool {
	return math.Abs(a-b) < 0.0001
}
func TestSpeedmapProc_ComputeDeltaTime(t *testing.T) {

	type args struct {
		trackPosCarInFront float64
		trackPosCurrentCar float64
	}
	tests := []struct {
		name string
		args args
		want float64
	}{
		{"equal position", args{0, 0}, 0},
		{"same chunk, cif just ahead current", args{0.05, 0.01}, 0.144},
		{"same chunk, cif right behind current", args{0.01, 0.05}, 3.456},
		{"0.3,0.5", args{0.3, 0.5}, 2.88},
		{"0.5,0.3", args{0.5, 0.3}, 0.72},
		{"cif first chunk, current last chunk", args{0.05, 0.95}, 0.36},
		{"cif last chunk, current first chunk", args{0.95, 0.05}, 3.24},
		{"tbd", args{0.0, 0.1}, 3.24},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := defaultTestSpeedmapProc()
			if got := s.ComputeDeltaTime(1, tt.args.trackPosCarInFront, tt.args.trackPosCurrentCar); !almostEqual(got, tt.want) {
				t.Errorf("SpeedmapProc.ComputeDeltaTime() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSpeedmapProc_computeLapTime(t *testing.T) {

	type args struct {
		trackPosCarInFront float64
		trackPosCurrentCar float64
	}
	tests := []struct {
		name string
		// args args
		want float64
	}{
		{"standard", 3.6},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := defaultTestSpeedmapProc()
			if got := s.computeLaptime(s.carClassLookup[1]); !almostEqual(got, tt.want) {
				t.Errorf("SpeedmapProc.computeLaptime() = %v, want %v", got, tt.want)
			}
		})
	}
}
