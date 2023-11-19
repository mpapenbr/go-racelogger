package processor

import (
	"fmt"
	"math"

	"github.com/mpapenbr/go-racelogger/log"
)

const (
	MarkerOverallBest  = "ob"
	MarkerClassBest    = "clb"
	MarkerCarBest      = "cb"
	MarkerPersonalBest = "pb"
	MarkerOldLap       = "old"
)

type TimeWithMarker struct {
	time   float64
	marker string
}

type ReportTimingStatus func(twm TimeWithMarker)

func (t *TimeWithMarker) String() string {
	formatMsg := func(marker string, time float64) string {
		return fmt.Sprintf("%s best lap %s", marker, formatLaptime(time))
	}
	switch t.marker {
	case MarkerOverallBest:
		return formatMsg("overall", t.time)
	case MarkerClassBest:
		return formatMsg("class", t.time)
	case MarkerCarBest:
		return formatMsg("car", t.time)
	case MarkerPersonalBest:
		return formatMsg("personal", t.time)
	}
	return ""
}

type CarLaptiming struct {
	lap     *SectionTiming
	sectors []*SectionTiming
}

type SectionTiming struct {
	startTime    float64
	stopTime     float64
	duration     TimeWithMarker
	personalBest float64
	inProgress   bool
	reportStatus ReportTimingStatus
}

func defaultSectionTiming() SectionTiming {
	return SectionTiming{startTime: -1, stopTime: -1, duration: TimeWithMarker{time: -1, marker: ""}, personalBest: math.MaxFloat64}
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
	if s.reportStatus != nil && marker != "" && marker != MarkerOldLap {
		s.reportStatus(s.duration)
	}
}

func (s *SectionTiming) isStarted() bool {
	return s.startTime != -1
}

func NewCarLaptiming(numSectors int, reportLapStatus ReportTimingStatus) *CarLaptiming {
	sectors := make([]*SectionTiming, numSectors)
	for i := range sectors {
		work := defaultSectionTiming()
		sectors[i] = &work
	}
	lap := defaultSectionTiming()
	lap.reportStatus = reportLapStatus

	return &CarLaptiming{
		lap:     &lap,
		sectors: sectors,
	}
}

// an argument of -1 means: don't evaluate
type (
	CollectCarLaptiming func(carClassId, carId int) []*CarLaptiming
	BestSectionProc     struct {
		sectors          []map[string]float64
		lap              map[string]float64
		collectFromOther CollectCarLaptiming
	}
)

func NewBestSectionProc(numSectors int, carClassIds, carIds []int, collector CollectCarLaptiming) *BestSectionProc {
	initData := func() map[string]float64 {
		ret := map[string]float64{}
		ret["overall"] = math.MaxFloat64

		for _, v := range carClassIds {
			ret[fmt.Sprintf("class%d", v)] = math.MaxFloat64
		}
		for _, v := range carIds {
			ret[fmt.Sprintf("car%d", v)] = math.MaxFloat64
		}
		return ret
	}
	sectors := make([]map[string]float64, numSectors)
	for i := range sectors {
		sectors[i] = initData()
	}
	return &BestSectionProc{
		sectors:          sectors,
		lap:              initData(),
		collectFromOther: collector,
	}
}

func (b *BestSectionProc) markSector(st *SectionTiming, numSector int, carClassId int, carId int) string {
	return b.markInternal(b.sectors[numSector], st, carClassId, carId,
		func(cl *CarLaptiming) *SectionTiming { return cl.sectors[numSector] },
	)
}

func (b *BestSectionProc) markLap(st *SectionTiming, carClassId int, carId int) string {
	return b.markInternal(b.lap, st, carClassId, carId,
		func(cl *CarLaptiming) *SectionTiming { return cl.lap })
}

func (b *BestSectionProc) markInternal(
	m map[string]float64,
	st *SectionTiming,
	carClassId, carId int,
	extractOther func(*CarLaptiming) *SectionTiming,
) string {
	className := fmt.Sprintf("class%d", carClassId)
	carName := fmt.Sprintf("car%d", carId)

	findWithMarker := func(other []*CarLaptiming, marker string) *SectionTiming {
		for i := range other {
			stOther := extractOther(other[i])
			if stOther.inProgress {
				continue
			}
			if stOther.duration.marker == marker {
				return stOther
			}
		}
		return nil
	}
	findEntryWithMarker := func(other []*CarLaptiming, marker string) *CarLaptiming {
		for i := range other {
			stOther := extractOther(other[i])
			if stOther.inProgress {
				continue
			}
			if stOther.duration.marker == marker {
				return other[i]
			}
		}
		return nil
	}

	handleDegrade := func(otherCar *SectionTiming, otherGeneric *SectionTiming, otherByCar []*CarLaptiming, marker string) {
		if otherCar != nil {
			otherCar.markDuration(MarkerPersonalBest)
			otherGeneric.markDuration(MarkerCarBest)
		} else {
			otherCL := findEntryWithMarker(otherByCar, marker)
			if otherCL != nil {
				otherGeneric.markDuration(MarkerPersonalBest)
			} else {
				otherGeneric.markDuration(MarkerCarBest)
			}

		}
	}
	if st.duration.time < 0 {
		log.Warn("early return from markInternal", log.Float64("time", st.duration.time))
		return st.duration.marker // keeping marker
	}

	if st.duration.time < m["overall"] {
		m["overall"] = st.duration.time
		m[className] = st.duration.time
		m[carName] = st.duration.time
		st.personalBest = st.duration.time
		st.inProgress = true
		other := b.collectFromOther(-1, -1)
		otherByClass := b.collectFromOther(carClassId, -1)
		otherByCar := b.collectFromOther(carClassId, carId)

		otherOverall := findWithMarker(other, MarkerOverallBest)
		otherClass := findWithMarker(otherByClass, MarkerClassBest)
		otherCar := findWithMarker(otherByCar, MarkerCarBest)

		// handle "degradation" of possible duplicates

		if otherOverall != nil {
			handleDegrade(otherCar, otherOverall, otherByCar, MarkerOverallBest)
		}
		if otherClass != nil {
			handleDegrade(otherCar, otherClass, otherByCar, MarkerClassBest)
		}

		if otherCar != nil {
			otherCar.markDuration(MarkerPersonalBest)
		}

		st.markDuration(MarkerOverallBest)
		st.inProgress = false

		return MarkerOverallBest
	}
	if st.duration.time < m[className] {
		m[className] = st.duration.time
		m[carName] = st.duration.time
		st.personalBest = st.duration.time
		st.inProgress = true
		otherByClass := b.collectFromOther(carClassId, -1)
		otherByCar := b.collectFromOther(carClassId, carId)

		otherClass := findWithMarker(otherByClass, MarkerClassBest)
		otherCar := findWithMarker(otherByCar, MarkerCarBest)

		if otherClass != nil {
			handleDegrade(otherCar, otherClass, otherByCar, MarkerClassBest)
		}

		if otherCar != nil {
			otherCar.markDuration(MarkerPersonalBest)
		}

		st.markDuration(MarkerClassBest)
		st.inProgress = false

		return MarkerClassBest
	}
	if st.duration.time < m[carName] {
		m[carName] = st.duration.time
		st.personalBest = st.duration.time
		st.inProgress = true
		otherByCar := b.collectFromOther(carClassId, carId)
		otherCar := findWithMarker(otherByCar, MarkerCarBest)
		if otherCar != nil {
			otherCar.markDuration(MarkerPersonalBest)
		}

		st.markDuration(MarkerCarBest)
		st.inProgress = false

		return MarkerCarBest
	}
	if st.duration.time < st.personalBest {
		st.personalBest = st.duration.time
		st.markDuration(MarkerPersonalBest)
		// remarkOthers(numSector, MarkerPersonalBest, MarkerClassBest)
		return MarkerPersonalBest
	}

	if st.duration.time > st.personalBest {
		st.markDuration("")
		return ""
	} else {
		return st.duration.marker
	}
}

func formatLaptime(t float64) string {
	work := t
	minutes := math.Floor(t / 60)
	work -= minutes * 60
	seconds := math.Trunc(work)
	work -= seconds
	hundreds := math.Trunc(work * 100)
	if minutes > 0 {
		return fmt.Sprintf("%.0f:%02d.%02d", minutes, int(seconds), int(hundreds))
	} else {
		return fmt.Sprintf("%02d.%02d", int(seconds), int(hundreds))
	}
}
