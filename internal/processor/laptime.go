package processor

const (
	MarkerOverallBest  = "ob"
	MarkerPersonalBest = "pb"
	MarkerClassBest    = "cb"
	MarkerOldLap       = "old"
)

type TimeWithMarker struct {
	time   float64
	marker string
}

type CarLaptiming struct {
	lap     SectionTiming
	sectors []SectionTiming
}

type SectionTiming struct {
	startTime float64
	stopTime  float64
	duration  TimeWithMarker
	best      float64
}

func defaultSectionTiming() SectionTiming {
	return SectionTiming{startTime: -1, stopTime: -1, duration: TimeWithMarker{time: -1, marker: ""}, best: -1}
}

func (s *SectionTiming) markStart(t float64) {
	s.startTime = t
}
func (s *SectionTiming) markStop(t float64) float64 {
	s.stopTime = t
	s.duration = TimeWithMarker{time: s.stopTime - s.startTime, marker: ""}
	return s.duration.time
}
func (s *SectionTiming) markDuration(marker string) {
	s.duration.marker = marker
}

func (s *SectionTiming) isStarted() bool {
	return s.startTime != -1
}

func NewCarLaptiming(numSectors int) *CarLaptiming {
	sectors := make([]SectionTiming, numSectors)
	for i := range sectors {
		sectors[i] = defaultSectionTiming()
	}
	return &CarLaptiming{
		lap:     defaultSectionTiming(),
		sectors: sectors}
}
