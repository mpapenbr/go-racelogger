package processor

import (
	"fmt"
	"math"

	speedmapv1 "buf.build/gen/go/mpapenbr/iracelog/protocolbuffers/go/iracelog/speedmap/v1"
	"github.com/mpapenbr/goirsdk/irsdk"
	"github.com/samber/lo"
	"golang.org/x/exp/slices"

	"github.com/mpapenbr/go-racelogger/log"
)

// collects speed data for a chunk of track
// we use this data to compute the current interval to another car
type ChunkData struct {
	id          int
	min         float64
	max         float64
	avg         float64
	history     []float64
	keepHistory int
	minHist     int
	ltSum       float64 // long term sum of recorded speeds
	ltCount     int     // long term count of recorded speeds
	ltAvg       float64 // long term average of recorded speeds
}

var speedThresholdPct float64 = 0.5

func newChunkData(id, keepHistory, minHist int) *ChunkData {
	return &ChunkData{
		id:          id,
		keepHistory: keepHistory,
		minHist:     minHist,
		history:     make([]float64, 0),
	}
}

func (p *ChunkData) update(speed float64) {
	if len(p.history) < p.keepHistory {
		p.history = append(p.history, speed)
		p.ltSum += speed
		p.ltCount++
		p.compute()
		return
	}
	// do not record speeds below the threshold
	if speed < p.ltAvg*speedThresholdPct {
		return
	}
	p.ltSum += speed
	p.ltCount++
	p.history = append(p.history, speed)
	if len(p.history)%2 == 1 {
		slices.Sort(p.history)
		p.history = p.history[1 : len(p.history)-2]
	}
	p.compute()
}

func (p *ChunkData) compute() {
	p.min = p.history[0]
	p.max = p.history[len(p.history)-1]
	p.avg = 0
	for _, v := range p.history {
		p.avg += v
	}
	p.avg /= float64(len(p.history))
	p.ltAvg = p.ltSum / float64(p.ltCount)
}

// SpeedmapProc is a struct that contains the logic to process the speedmap data.
// It is used by the Processor struct.

type SpeedmapProc struct {
	api            *irsdk.Irsdk
	chunkSize      int
	gpd            *GlobalProcessingData
	numChunks      int
	leaderTrackPos float64
	carClassLookup map[int][]*ChunkData // car class id -> chunk data
	carIdLookup    map[int][]*ChunkData // car id -> chunk data
	carLookup      map[int][]*ChunkData // car idx -> chunk data
}

func SetSpeedmapSpeedThreshold(pct float64) {
	speedThresholdPct = pct
}

//nolint:whitespace // can't get different linters happy
func NewSpeedmapProc(
	api *irsdk.Irsdk,
	chunkSize int,
	gpd *GlobalProcessingData,
) *SpeedmapProc {
	return &SpeedmapProc{
		api:            api,
		chunkSize:      chunkSize,
		numChunks:      int(math.Ceil(float64(gpd.TrackInfo.Length) / float64(chunkSize))),
		carClassLookup: make(map[int][]*ChunkData),
		carIdLookup:    make(map[int][]*ChunkData),
		carLookup:      make(map[int][]*ChunkData),
		gpd:            gpd,
	}
}

func (s *SpeedmapProc) Process(carData *CarData, carClassId, carId int) {
	s.ensureLookup(s.carLookup, int(carData.carIdx))
	s.ensureLookup(s.carClassLookup, carClassId)
	s.ensureLookup(s.carLookup, carId)
	idx := s.getChunkIdx(carData.trackPos)
	s.carLookup[int(carData.carIdx)][idx].update(carData.speed)
	s.carClassLookup[carClassId][idx].update(carData.speed)
	s.carLookup[carId][idx].update(carData.speed)
}

func (s *SpeedmapProc) SetLeaderTrackPos(trackPos float64) {
	s.leaderTrackPos = trackPos
}

//nolint:lll,funlen,whitespace // better readability
func (s *SpeedmapProc) ComputeDeltaTime(
	carClassId int, trackPosCarInFront, trackPosCurrentCar float64,
) float64 {
	idxCarInFront := s.getChunkIdx(trackPosCarInFront)
	idxCurrentCar := s.getChunkIdx(trackPosCurrentCar)
	if _, ok := s.carClassLookup[carClassId]; !ok {
		log.Warn("No chunk data for car class", log.Int("carClassId", carClassId))
		return -1
	}

	// chunkData should contain all chunks from currentCar to carInFront
	// Example: 6 chunks
	// current=1, cif=4 => chunks 4,5,0,1
	// current=4, cif=1 => chunks 1,2,3,4
	// chunk[0] is traveled from trackPos to EndOfChunk
	// chunk[last] is traveled from StartOfChunk to trackPos
	chunkData := make([]*ChunkData, 0)
	if trackPosCarInFront < trackPosCurrentCar {
		chunkData = append(chunkData, s.carClassLookup[carClassId][idxCurrentCar:]...)
		chunkData = append(chunkData, s.carClassLookup[carClassId][0:idxCarInFront+1]...)
	} else {
		chunkData = append(chunkData, s.carClassLookup[carClassId][idxCurrentCar:idxCarInFront+1]...)
	}
	if len(chunkData) == 0 {
		return 0
	}
	if slices.ContainsFunc(chunkData, func(cd *ChunkData) bool {
		return cd.avg == 0
	}) {
		return 0
	}
	// corner case: only one chunk, check distance between cars
	// (carInFront is ahead of currentCar)
	if len(chunkData) == 1 {
		distMeters := (trackPosCarInFront - trackPosCurrentCar) * float64(s.gpd.TrackInfo.Length)
		delta := distMeters / chunkData[0].avg * 3.6
		return delta
	}
	// for the first item: calculate the time from trackPosCurrentCar to end of chunk
	metersToEndOfChunk := float64((idxCurrentCar+1)*s.chunkSize) -
		(trackPosCurrentCar * float64(s.gpd.TrackInfo.Length))
	delta := metersToEndOfChunk / chunkData[0].avg * 3.6
	totalDelta := delta

	// collect the chunks between the two cars
	if len(chunkData) > 1 {
		for _, c := range chunkData[1 : len(chunkData)-1] {
			totalDelta += float64(s.chunkSize) / c.avg * 3.6
		}
	}

	// for the last item: calculate the time from start of chunk to trackPosCarInFront
	metersFromStartOfChunk := trackPosCarInFront*float64(s.gpd.TrackInfo.Length) -
		(float64(idxCarInFront * s.chunkSize))
	delta = metersFromStartOfChunk / chunkData[len(chunkData)-1].avg * 3.6
	totalDelta += delta
	return totalDelta
}

func (s *SpeedmapProc) CreatePayload() *speedmapv1.Speedmap {
	content := s.CreateOutput()
	ret := &speedmapv1.Speedmap{
		ChunkSize:      uint32(s.chunkSize),
		Data:           content,
		SessionTime:    float32(readFloat64(s.api, "SessionTime")),
		TimeOfDay:      uint32(readFloat32(s.api, "SessionTimeOfDay")),
		TrackTemp:      readFloat32(s.api, "TrackTemp"),
		LeaderTrackPos: float32(s.leaderTrackPos),
	}
	return ret
}

func (s *SpeedmapProc) CreateOutput() map[string]*speedmapv1.ClassData {
	ret := make(map[string]*speedmapv1.ClassData)
	for k, v := range s.carClassLookup {
		laptime := s.computeLaptime(v)
		chunkSpeeds := lo.Map(v, func(cd *ChunkData, _ int) float32 {
			return float32(cd.avg)
		})
		ret[fmt.Sprintf("%d", k)] = &speedmapv1.ClassData{
			Laptime:     float32(laptime),
			ChunkSpeeds: chunkSpeeds,
		}
	}
	return ret
}

func (s *SpeedmapProc) computeLaptime(chunks []*ChunkData) float64 {
	if !s.hasValidAvgs(chunks) {
		return 0
	}

	ret := lo.Reduce(chunks, func(acc float64, cd *ChunkData, _ int) float64 {
		return acc + float64(s.chunkSize)/cd.avg*3.6
	}, 0)
	return ret
}

func (s *SpeedmapProc) hasValidAvgs(chunks []*ChunkData) bool {
	return !slices.ContainsFunc(chunks, func(cd *ChunkData) bool {
		return cd.avg == 0
	})
}

func (s *SpeedmapProc) ensureLookup(lookup map[int][]*ChunkData, id int) {
	if _, ok := lookup[id]; !ok {
		lookup[id] = make([]*ChunkData, s.numChunks)
		for i := 0; i < s.numChunks; i++ {
			lookup[id][i] = newChunkData(i, 11, 3)
		}
	}
}

//nolint:lll //  readability
func (s *SpeedmapProc) getChunkIdx(trackPos float64) int {
	if trackPos > 1 {
		return 0
	} else {
		idx := int(math.Floor(trackPos * float64(s.gpd.TrackInfo.Length) / float64(s.chunkSize)))
		if idx >= s.numChunks {
			return idx - 1
		}
		return idx
	}
}
