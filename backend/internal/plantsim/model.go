package plantsim

import (
	"fmt"
	"math"
	"math/rand/v2"
	"sync"
	"time"
)

// Model is the physics engine of the stand (TZ §8). It advances in fixed
// ticks; every tick refreshes the register image the Modbus server serves.
// All states are initialised at the nominal steady point of TZ §5 so a fresh
// stand starts in a credible operating regime.
//
// Documented demo compressions (TZ §19): flotation residence time is ~7x
// faster than a real bank, and the thickener bed accumulates 300x faster —
// otherwise a pump-failure upset would take hours to become visible.
const (
	thickenerCompression = 120.0
	binCapacityTPH       = 200.0
	flocculantSetpoint   = 12.0 // g/t, fixed plant setting (fi401)
)

// Model holds the full process state.
type Model struct {
	mu sync.Mutex

	rng *rand.Rand

	// Holding-register (actuator) image. FC6 writes land here directly;
	// the model reads them every tick.
	holding [NumHoldingRegisters]uint16

	// Input-register (sensor) image, refreshed every tick.
	input [NumInputRegisters]uint16

	// --- Ore feed (§8.1) ---
	binTPH       float64 // ore in the bin, t
	feedTPH      float64 // fi101 true (belt state)
	hardness     float64 // H, OU process
	hardOffset   float64 // scenario 1 persistent step
	replenishTPH float64

	// --- Grinding (§8.2) ---
	circLoad    float64 // CL, fraction (2.5 nominal)
	millPowerKW float64
	cyclDensity float64 // di202 true, g/l
	cyclPress   float64 // pi201 true, kPa
	p80         float64 // xi201 true, um
	p80Frozen   bool    // scenario 3

	// --- Flotation (§8.3) ---
	levelMM   float64 // li301 true
	pH        float64 // true pH
	phDipAmp  float64 // scenario 5 dip depth (0 = none)
	phDipT0   time.Time
	phDipDur  time.Duration
	collector float64 // qi301 true
	frother   float64 // qi302 true
	oreFactor float64 // 1.0 sulfide / 0.86 mixed / 0.72 oxide

	concTPH   float64 // Wc smoothed (dry conc production)
	concGrade float64 // beta smoothed

	// XRF layer
	xrfAlpha      float64 // true feed grade (slow walk)
	xrfDrift      bool    // scenario 2
	xrfDrift0     time.Time
	xrfHold       time.Time
	afi, afc, aft float64 // published (sample&hold) grades
	xiMeas        float64 // published P80 (sensor)

	// --- Thickening (§8.4) ---
	bedMass   float64 // M, t
	ufDensity float64 // di401 true, %sol
	rakeTorq  float64 // ei401 true, %
	ufPumpOff bool    // scenario 6
	ufPumpT0  time.Time
	ufPumpDur time.Duration

	// --- Filtration (§8.5) ---
	vacuumKPa float64
	cakeMoist float64
	cakeTPH   float64

	// --- Misc ---
	pulpTempC float64

	// commLoss is scenario 4: the Modbus server accepts connections but
	// never answers (the client times out), emulating a dead device.
	commLoss bool

	// Diagnostics
	tickCount int64
	clock     time.Time // simulated clock (starts at real time, advances by dt)
}

// New creates the model at the nominal steady state (TZ §5/§8): 100 t/h feed,
// CL 250 %, level 500 mm, pH 10.2, bed 3.0 m, beta 22 %, theta 0.08 %.
func New(seed uint64, now time.Time) *Model {
	m := &Model{rng: rand.New(rand.NewPCG(seed, seed)), clock: now}
	m.holding[RegHC101] = enc(100.0, holdingScales[RegHC101])
	m.holding[RegFC201] = enc(55.0, holdingScales[RegFC201])
	m.holding[RegFC301] = enc(180, holdingScales[RegFC301])
	m.holding[RegFC302] = enc(60, holdingScales[RegFC302])
	m.holding[RegLC301] = enc(40.0, holdingScales[RegLC301])
	m.holding[RegFC401] = enc(45.0, holdingScales[RegFC401])
	m.holding[RegHI501] = enc(45, holdingScales[RegHI501])

	m.binTPH = 100.0
	m.feedTPH = 100.0
	m.replenishTPH = 100.0
	m.hardness = 1.0
	m.circLoad = 2.5
	m.millPowerKW = 1250
	m.cyclDensity = 1520
	m.cyclPress = 118
	m.p80 = 150
	m.xiMeas = 150
	m.levelMM = 500
	m.pH = 10.2
	m.collector = 180
	m.frother = 60
	m.oreFactor = 1.0
	m.concTPH = 3.29
	m.concGrade = 22.0
	m.xrfAlpha = 0.80
	m.afi, m.afc, m.aft = 0.80, 22.0, 0.08
	m.bedMass = 119.0
	m.ufDensity = 45.0
	m.rakeTorq = 55.0
	m.vacuumKPa = 62
	m.cakeMoist = 10.5
	m.cakeTPH = 2.94
	m.pulpTempC = 20.0

	m.Tick(0) // build the initial sensor image from the nominal state
	return m
}

// enc converts an engineering value to a raw register count.
func enc(v, scale float64) uint16 {
	raw := math.Round(v / scale)
	if raw < 0 {
		raw = 0
	}
	if raw > 65535 {
		raw = 65535
	}
	return uint16(raw)
}

func dec(raw uint16, scale float64) float64 { return float64(raw) * scale }

// lag moves x toward target with first-order dynamics of time constant tau.
func lag(x, target, tau, dt float64) float64 {
	if tau <= 0 {
		return target
	}
	return x + (target-x)*(dt/tau)
}

func clampf(v, lo, hi float64) float64 { return math.Min(math.Max(v, lo), hi) }

// percentSolids converts pulp density (g/l) to % solids by mass (TZ §10.3).
func percentSolids(pulpGL, solidsSG float64) float64 {
	rhoP := pulpGL / 1000.0
	if rhoP <= 1.0 || solidsSG <= 1.0 {
		return 0
	}
	return solidsSG * (rhoP - 1.0) / ((solidsSG - 1.0) * rhoP) * 100
}

// Tick advances the model by dt (TZ §8). The Modbus server calls it on the
// configured cadence.
func (m *Model) Tick(dt time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	d := dt.Seconds()
	m.tickCount++
	m.clock = m.clock.Add(dt)

	// --- Actuators (holding registers) ---
	feederSP := dec(m.holding[RegHC101], holdingScales[RegHC101])
	waterValve := dec(m.holding[RegFC201], holdingScales[RegFC201])
	collectorSP := dec(m.holding[RegFC301], holdingScales[RegFC301])
	frotherSP := dec(m.holding[RegFC302], holdingScales[RegFC302])
	tailgate := dec(m.holding[RegLC301], holdingScales[RegLC301])
	ufPump := dec(m.holding[RegFC401], holdingScales[RegFC401])
	filterCycle := dec(m.holding[RegHI501], holdingScales[RegHI501])

	// --- Scenario clocks ---
	if m.phDipAmp > 0 && m.clock.Sub(m.phDipT0) > m.phDipDur {
		m.phDipAmp = 0
	}
	ufPumpEff := ufPump
	if m.ufPumpOff {
		if m.clock.Sub(m.ufPumpT0) > m.ufPumpDur {
			m.ufPumpOff = false
		} else {
			ufPumpEff = 0
		}
	}

	// --- §8.1 Ore feed ---
	// Hardness: Ornstein–Uhlenbeck around 1.0 (tau 20 min, stationary SD
	// 0.012) + scenario step. Keeps the quiet-running recovery inside the
	// 88–92 % acceptance corridor while still wandering on trends.
	ou := m.rng.NormFloat64() * 0.012 * math.Sqrt(2*d/1200.0)
	m.hardness = clampf(m.hardness+(1.0-m.hardness)*(d/1200.0)+ou, 0.8, 1.2)
	hEff := clampf(m.hardness+m.hardOffset, 0.8, 1.2)

	m.binTPH = clampf(m.binTPH+(m.replenishTPH-feederSP)*d/3600, 0, binCapacityTPH)
	m.feedTPH = lag(m.feedTPH, feederSP, 8, d) // belt inertia toward the setpoint
	if m.binTPH <= 0.01 {
		m.feedTPH = lag(m.feedTPH, 0, 8, d) // empty bin starves the mill
	}
	fi101 := clampf(m.feedTPH, 0, 200)
	ei101 := 1.5 * fi101 * hEff

	// --- §8.2 Grinding / classification ---
	clTarget := 2.5 * math.Pow(hEff, 1.5)
	m.circLoad = lag(m.circLoad, clTarget, 120, d)
	m.millPowerKW = lag(m.millPowerKW, 950+120*m.circLoad+300*(fi101-100)/100, 2, d)
	m.millPowerKW = clampf(m.millPowerKW, 0, 2500)

	densTarget := 1520 + 320*(waterValve-55)/55
	m.cyclDensity = clampf(lag(m.cyclDensity, densTarget, 20, d), 1000, 2000)
	pressTarget := 118 * math.Sqrt(clampf(waterValve, 5, 100)/55)
	m.cyclPress = clampf(lag(m.cyclPress, pressTarget, 10, d), 0, 350)
	p80Target := 150 * math.Sqrt(m.cyclPress/118) * math.Pow(m.cyclDensity/1520, 1.5) * hEff
	m.p80 = clampf(lag(m.p80, p80Target, 30, d), 40, 300)

	// Pulp flow to the cyclone: fresh feed x (1+CL) in dry solids, converted
	// through the cyclone density.
	solids := percentSolids(m.cyclDensity, 2.7) / 100
	fi202 := fi101 * (1 + m.circLoad) / (solids * m.cyclDensity / 1000)
	fi202 = clampf(fi202, 0, 600)

	// --- §8.3 Flotation ---
	qIn := 250.0
	qOut := 6.25 * tailgate * math.Sqrt(m.levelMM/500) // 625 at 100 %
	m.levelMM = clampf(m.levelMM+(qIn-qOut)/12.0*1000*d/3600, 200, 800)

	phNow := m.pH
	if m.phDipAmp > 0 {
		elapsed := m.clock.Sub(m.phDipT0).Seconds()
		// Dip to the target over ~30 s, then exponential recovery.
		dip := m.phDipAmp * math.Min(1, elapsed/30)
		phNow = 10.2 - dip + m.phDipAmp*(1-math.Exp(-math.Max(0, elapsed-30)/120))
	}
	m.pH = phNow

	m.collector = lag(m.collector, collectorSP, 6, d)
	m.frother = lag(m.frother, frotherSP, 6, d)

	// Kinetics (first order, Wills ch.12; calibrated per TZ §8.3).
	grind := clampf((240-m.p80)/90, 0.25, 1.15)
	dose := 1.2 * m.collector / (m.collector + 36)
	phFactor := math.Exp(-math.Pow((m.pH-10.2)/1.4, 2))
	rInf := 0.97 * grind * dose * phFactor * m.oreFactor
	recovery := math.Min(rInf*(1-math.Exp(-0.9*3)), 0.985)

	// Mass transfer: fixed point over (Wc, beta) with a pull-quality tradeoff.
	wc, beta := m.concTPH, m.concGrade
	if fi101 > 1 && m.xrfAlpha > 0 {
		for i := 0; i < 5; i++ {
			gamma := wc / fi101
			betaT := 22 * math.Exp(-math.Pow((m.pH-10.2)/2.2, 2)) *
				(1 - 0.35*math.Max(0, (gamma-0.035)/0.06))
			betaT = clampf(betaT, 5, 30)
			wc = recovery * fi101 * m.xrfAlpha / betaT
		}
		// Over-pulling erodes recovery (entrainment).
		gamma := wc / fi101
		recovery *= 1 - 0.2*math.Max(0, (gamma-0.05)/0.05)
		recovery = clampf(recovery, 0, 0.985)
	}
	// Smooth toward the fixed point (tau 45 s).
	m.concTPH = lag(m.concTPH, wc, 45, d)
	m.concGrade = lag(m.concGrade, beta, 45, d)

	tailsTPH := fi101 - m.concTPH
	theta := 0.0
	if fi101 > m.concTPH {
		theta = m.xrfAlpha * (1 - recovery) / (1 - m.concTPH/fi101)
	}
	theta = clampf(theta, 0.01, 0.5)

	// XRF layer: slow feed-grade walk + sample&hold every 60 s.
	ou2 := m.rng.NormFloat64() * 0.05 * math.Sqrt(2*d/600.0)
	m.xrfAlpha = clampf(m.xrfAlpha+(1.0-m.xrfAlpha)*(d/600)+ou2, 0.65, 0.95)
	if m.clock.Sub(m.xrfHold) >= 60*time.Second {
		m.xrfHold = m.clock
		alpha := m.xrfAlpha
		if m.xrfDrift {
			alpha += 0.00002 * m.clock.Sub(m.xrfDrift0).Seconds()
		}
		m.afi = clampf(alpha+m.rng.NormFloat64()*0.01, 0.1, 2.0)
		m.afc = clampf(m.concGrade+m.rng.NormFloat64()*0.15, 5, 30)
		m.aft = clampf(theta+m.rng.NormFloat64()*0.004, 0.01, 0.5)
	}
	if !m.p80Frozen {
		m.xiMeas = m.p80
	}

	// --- §8.4 Thickening ---
	ufDry := 12.0 * (ufPumpEff / 100) * 1.38 * (m.ufDensity / 100) // t/h dry
	bedRate := (m.concTPH - ufDry) * thickenerCompression / 3600   // t per second
	m.bedMass = clampf(m.bedMass+bedRate, 0, 316)                  // 316 t = 8 m bed
	ufTarget := 45 + 15*math.Tanh(m.bedMass/119-1) + 4*(flocculantSetpoint/12-1)
	m.ufDensity = lag(m.ufDensity, clampf(ufTarget, 20, 70), 60, d)
	m.rakeTorq = lag(m.rakeTorq, clampf(15+40*math.Pow(m.bedMass/119, 2), 0, 100), 10, d)

	// --- §8.5 Filtration ---
	m.vacuumKPa = lag(m.vacuumKPa, 62*math.Pow(clampf(filterCycle, 10, 120)/45, 0.3), 25, d)
	moistTarget := 10.5 + 0.06*(45-filterCycle) + 0.15*(m.ufDensity-45)/10
	m.cakeMoist = lag(m.cakeMoist, clampf(moistTarget, 4, 25), 60, d)
	ufDrySmooth := ufDry
	m.cakeTPH = lag(m.cakeTPH, clampf(ufDrySmooth*(1-m.cakeMoist/100), 0, 10), 30, d)

	// --- Pulp temperature: slow ambient drift ---
	m.pulpTempC = lag(m.pulpTempC, 20, 600, d)

	// --- Air flow (uncontrolled) ---
	airFlow := 350 + 10*math.Sin(float64(m.tickCount)/600)

	// --- di301: cyclone overflow diluted to the flotation feed (~30 % sol) ---
	di301 := 30 + 6*(m.cyclDensity-1520)/320
	di301 = clampf(di301, 10, 45)

	// --- Sensor image (§8.6): quantize; the raw lag dynamics above already
	// carry the process transients, the small jitter below is measurement
	// noise within the quantisation step. ---
	m.input[RegFI101] = enc(fi101+jitter(m.rng, 0.05), inputScales[RegFI101])
	m.input[RegEI101] = enc(ei101+jitter(m.rng, 0.5), inputScales[RegEI101])
	m.input[RegEI201] = enc(m.millPowerKW+jitter(m.rng, 1.0), inputScales[RegEI201])
	m.input[RegFI201] = enc(waterValve*3+jitter(m.rng, 0.1), inputScales[RegFI201])
	m.input[RegPI201] = enc(m.cyclPress+jitter(m.rng, 0.1), inputScales[RegPI201])
	m.input[RegDI202] = enc(m.cyclDensity, inputScales[RegDI202])
	m.input[RegXI201] = enc(m.xiMeas, inputScales[RegXI201])
	m.input[RegFI202] = enc(fi202+jitter(m.rng, 0.1), inputScales[RegFI202])
	m.input[RegLI301] = enc(m.levelMM, inputScales[RegLI301])
	m.input[RegFI301] = enc(airFlow, inputScales[RegFI301])
	m.input[RegAI301] = enc(phNow+jitter(m.rng, 0.002), inputScales[RegAI301])
	m.input[RegQI301] = enc(m.collector, inputScales[RegQI301])
	m.input[RegQI302] = enc(m.frother, inputScales[RegQI302])
	m.input[RegDI301] = enc(di301, inputScales[RegDI301])
	m.input[RegAFI301] = enc(m.afi, inputScales[RegAFI301])
	m.input[RegAFC301] = enc(m.afc, inputScales[RegAFC301])
	m.input[RegAFT301] = enc(m.aft, inputScales[RegAFT301])
	m.input[RegWI301] = enc(m.concTPH+jitter(m.rng, 0.002), inputScales[RegWI301])
	m.input[RegWI302] = enc(tailsTPH+jitter(m.rng, 0.02), inputScales[RegWI302])
	m.input[RegLI401] = enc(m.bedMass/39.62, inputScales[RegLI401])
	m.input[RegDI401] = enc(m.ufDensity, inputScales[RegDI401])
	m.input[RegEI401] = enc(m.rakeTorq, inputScales[RegEI401])
	m.input[RegFI401] = enc(flocculantSetpoint+jitter(m.rng, 0.02), inputScales[RegFI401])
	m.input[RegPI501] = enc(m.vacuumKPa+jitter(m.rng, 0.02), inputScales[RegPI501])
	m.input[RegMI501] = enc(m.cakeMoist, inputScales[RegMI501])
	m.input[RegWI501] = enc(m.cakeTPH, inputScales[RegWI501])
	m.input[RegTIT101] = enc(m.pulpTempC+jitter(m.rng, 0.02), inputScales[RegTIT101])
	m.input[RegSI101] = enc(m.binTPH/binCapacityTPH*100, inputScales[RegSI101])
}

// jitter returns a small zero-mean measurement noise.
func jitter(r *rand.Rand, sigma float64) float64 {
	if sigma == 0 {
		return 0
	}
	return r.NormFloat64() * sigma
}

// Scenario applies a scenario command (TZ §7.3). Returns a description.
func (m *Model) Scenario(code int, value int) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	switch code {
	case ScenarioNormal:
		m.hardOffset = 0
		m.xrfDrift = false
		m.p80Frozen = false
		m.phDipAmp = 0
		m.ufPumpOff = false
		m.oreFactor = 1.0
		m.commLoss = false
		return "норма: возмущения сброшены"
	case ScenarioOreHard:
		m.hardOffset = clampf(float64(value)/100.0, -0.4, 0.4)
		return fmt.Sprintf("крепость руды %+v%%", value)
	case ScenarioXRFFDrift:
		if value == 1 {
			m.xrfDrift = true
			m.xrfDrift0 = m.clock
			return "дрейф XRF питания включён"
		}
		m.xrfDrift = false
		return "дрейф XRF питания выключен"
	case ScenarioP80Freeze:
		if value == 1 {
			m.p80Frozen = true
			return "датчик P80 заморожен"
		}
		m.p80Frozen = false
		return "датчик P80 разморожен"
	case ScenarioCommLoss:
		if value == 1 {
			m.commLoss = true
			return "обрыв связи включён"
		}
		m.commLoss = false
		return "обрыв связи выключен"
	case ScenarioPHDip:
		m.phDipAmp = 1.4 // pH 10.2 -> 8.8
		m.phDipT0 = m.clock
		m.phDipDur = time.Duration(value) * time.Second
		return fmt.Sprintf("закисление пульпы на %d с", value)
	case ScenarioUFPumpFail:
		m.ufPumpOff = true
		m.ufPumpT0 = m.clock
		m.ufPumpDur = time.Duration(value) * time.Second
		return fmt.Sprintf("отказ насоса сгустителя на %d с", value)
	case ScenarioBinFill:
		m.binTPH = clampf(m.binTPH+float64(value)/100*binCapacityTPH, 0, binCapacityTPH)
		return fmt.Sprintf("бункер пополнен на %d%%", value)
	case ScenarioOreType:
		switch value {
		case 1:
			m.oreFactor = 0.86
			return "руда: смешанная"
		case 2:
			m.oreFactor = 0.72
			return "руда: окисленная"
		default:
			m.oreFactor = 1.0
			return "руда: сульфидная"
		}
	default:
		return fmt.Sprintf("неизвестный сценарий %d", code)
	}
}

// CommLoss reports whether scenario 4 requested a Modbus outage.
func (m *Model) CommLoss() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.commLoss
}

// Input returns a copy of the FC4 image.
func (m *Model) Input() [NumInputRegisters]uint16 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.input
}

// Holding returns a copy of the FC3 image.
func (m *Model) Holding() [NumHoldingRegisters]uint16 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.holding
}

// WriteHolding applies an FC6 write with range validation. Returns the
// applied raw value or an error when out of the documented range.
func (m *Model) WriteHolding(addr uint16, value uint16) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if addr >= NumHoldingRegisters {
		return fmt.Errorf("address %d out of range", addr)
	}
	lim := holdingWriteLimits[addr]
	if value < lim[0] || value > lim[1] {
		return fmt.Errorf("value %d out of range [%d, %d] for HR%d", value, lim[0], lim[1], addr)
	}
	m.holding[addr] = value
	return nil
}

// State is the diagnostic dump for GET /state.
type State struct {
	Tick       int64   `json:"tick"`
	Hardness   float64 `json:"hardness"`
	CircLoad   float64 `json:"circ_load"`
	P80        float64 `json:"p80_um"`
	LevelMM    float64 `json:"level_mm"`
	PH         float64 `json:"ph"`
	ConcTPH    float64 `json:"conc_tph"`
	ConcGrade  float64 `json:"conc_grade_pct"`
	BedMass    float64 `json:"bed_mass_t"`
	UFDensity  float64 `json:"uf_density_pct"`
	RakeTorque float64 `json:"rake_torque_pct"`
	BinPercent float64 `json:"bin_percent"`
	OreFactor  float64 `json:"ore_factor"`
}

// State returns the internal state snapshot (diagnostics only).
func (m *Model) State() State {
	m.mu.Lock()
	defer m.mu.Unlock()
	return State{
		Tick: m.tickCount, Hardness: m.hardness + m.hardOffset,
		CircLoad: m.circLoad, P80: m.p80, LevelMM: m.levelMM, PH: m.pH,
		ConcTPH: m.concTPH, ConcGrade: m.concGrade,
		BedMass: m.bedMass, UFDensity: m.ufDensity, RakeTorque: m.rakeTorq,
		BinPercent: m.binTPH / binCapacityTPH * 100, OreFactor: m.oreFactor,
	}
}
