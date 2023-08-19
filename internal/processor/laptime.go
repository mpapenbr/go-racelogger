package processor

import (
	"fmt"
	"math"
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
}

func (s *SectionTiming) isStarted() bool {
	return s.startTime != -1
}

func NewCarLaptiming(numSectors int) *CarLaptiming {
	sectors := make([]*SectionTiming, numSectors)
	for i := range sectors {
		work := defaultSectionTiming()
		sectors[i] = &work
	}
	lap := defaultSectionTiming()
	return &CarLaptiming{
		lap:     &lap,
		sectors: sectors}
}

// an argument of -1 means: don't evaluate
type CollectCarLaptiming func(carClassId, carId int) []*CarLaptiming
type BestSectionProc struct {
	sectors          []map[string]float64
	lap              map[string]float64
	collectFromOther CollectCarLaptiming
}

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

	// degradeOther := func(otherCar *SectionTiming, otherGeneric *SectionTiming, otherByCar []CarLaptiming) {
	// 	if otherCar != nil {
	// 		otherCar.markDuration(MarkerPersonalBest)
	// 		otherGeneric.markDuration(MarkerCarBest)
	// 	} else {
	// 		if currentIsInSameCar(otherByCar) {
	// 			otherGeneric.markDuration(MarkerPersonalBest)
	// 		} else {
	// 			otherGeneric.markDuration(MarkerCarBest)
	// 		}
	// 	}
	// }

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
			if otherCar != nil {
				otherCar.markDuration(MarkerPersonalBest)
				otherOverall.markDuration(MarkerCarBest)
			} else {
				otherCLOverall := findEntryWithMarker(otherByCar, MarkerOverallBest)
				if otherCLOverall != nil {
					otherOverall.markDuration(MarkerPersonalBest)
				} else {
					otherOverall.markDuration(MarkerCarBest)
				}

			}
		}

		if otherClass != nil {
			if otherCar != nil {
				otherCar.markDuration(MarkerPersonalBest)
				otherClass.markDuration(MarkerCarBest)
			} else {
				otherCLClassBest := findEntryWithMarker(otherByCar, MarkerClassBest)
				if otherCLClassBest != nil {
					otherClass.markDuration(MarkerPersonalBest)
				} else {
					otherClass.markDuration(MarkerCarBest)
				}
			}
		}
		if otherCar != nil {
			otherCar.markDuration(MarkerPersonalBest)
		}

		st.markDuration(MarkerOverallBest)
		st.inProgress = false
		// remarkOthers(numSector, MarkerOverallBest, MarkerClassBest)
		return MarkerOverallBest
	}
	if st.duration.time < m[className] {
		m[className] = st.duration.time
		m[carName] = st.duration.time
		st.personalBest = st.duration.time
		st.markDuration(MarkerClassBest)
		// remarkOthers(numSector, MarkerPersonalBest, MarkerClassBest)
		return MarkerClassBest
	}
	if st.duration.time < m[carName] {
		m[carName] = st.duration.time
		st.personalBest = st.duration.time
		st.markDuration(MarkerCarBest)
		// remarkOthers(numSector, MarkerPersonalBest, MarkerClassBest)
		return MarkerCarBest
	}
	if st.duration.time < st.personalBest {
		st.personalBest = st.duration.time
		st.markDuration(MarkerPersonalBest)
		// remarkOthers(numSector, MarkerPersonalBest, MarkerClassBest)
		return MarkerPersonalBest
	} else {
		st.markDuration("")
		return ""
	}
}
