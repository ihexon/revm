package probes

type Registers struct {
	Probes []ServiceProber
}

func NewRegisters() *Registers {
	return &Registers{}
}

func (r *Registers) AddProbe(probe ...ServiceProber) {
	for _, p := range probe {
		r.Probes = append(r.Probes, p)
	}
}

func (r *Registers) GetProbes() []ServiceProber {
	return r.Probes
}


