package probes

type Registers struct {
	Probes []ServiceProber
}

func NewRegisters() *Registers {
	return &Registers{}
}

func (r *Registers) AddProbe(probe ...ServiceProber) {
	r.Probes = append(r.Probes, probe...)
}

func (r *Registers) GetProbes() []ServiceProber {
	return r.Probes
}
