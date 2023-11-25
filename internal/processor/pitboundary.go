package processor

import "golang.org/x/exp/slices"

type PitBoundaryData struct {
	min         float64
	max         float64
	middle      float64
	minHistory  int
	keepHistory int
	history     []float64
	computed    bool
}

func (p *PitBoundaryData) update(trackPos float64) {
	if len(p.history) < p.keepHistory {
		p.history = append(p.history, trackPos)
		p.compute()
		return
	}
	p.history = append(p.history, trackPos)
	if len(p.history)%2 == 1 {
		slices.Sort(p.history)
		p.history = p.history[1 : len(p.history)-2]
	}
}

func (p *PitBoundaryData) compute() {
	p.min = p.history[0]
	p.max = p.history[len(p.history)-1]
	p.middle = p.history[len(p.history)>>1]
	p.computed = true
}

type PitBoundaryProc struct {
	pitEntry PitBoundaryData
	pitExit  PitBoundaryData
}

func NewPitBoundaryProc() *PitBoundaryProc {
	return &PitBoundaryProc{
		pitEntry: PitBoundaryData{minHistory: 3, keepHistory: 21},
		pitExit:  PitBoundaryData{minHistory: 3, keepHistory: 21},
	}
}

func (p *PitBoundaryProc) processPitEntry(trackPos float64) {
	p.pitEntry.update(trackPos)
}

func (p *PitBoundaryProc) processPitExit(trackPos float64) {
	p.pitExit.update(trackPos)
}
