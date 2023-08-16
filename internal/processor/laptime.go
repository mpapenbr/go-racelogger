package processor

type LaptimeProc struct {
	numSectors int
}

type SectorProc struct {
}

func NewLaptimeProc(numSectors int) *LaptimeProc {
	return &LaptimeProc{numSectors: numSectors}
}
