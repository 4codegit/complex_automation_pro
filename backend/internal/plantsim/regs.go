// Package plantsim is the process demonstration stand (TZ §7/§8): a physics
// model of the flotation plant exposed through a real Modbus TCP server. The
// platform never talks to this package in-process — the edge gateway polls it
// over Modbus exactly as it would poll a plant PLC.
package plantsim

// Register areas of the stand (TZ §7). All values are u16 raw counts; the
// engineering value is raw × Scale.
const (
	// NumInputRegisters is the size of the FC4 area (sensors).
	NumInputRegisters = 28
	// NumHoldingRegisters is the size of the FC3/FC6 area (actuators + scenario).
	NumHoldingRegisters = 9
)

// Input register offsets (FC4).
const (
	RegFI101  = 0  // feeder rate, t/h, scale 0.1
	RegEI101  = 1  // crusher power, kW, scale 0.1
	RegEI201  = 2  // mill power, kW, scale 0.1
	RegFI201  = 3  // fresh mill water, m3/h, scale 0.1
	RegPI201  = 4  // cyclone pressure, kPa, scale 0.1
	RegDI202  = 5  // cyclone overflow density, g/l, scale 1
	RegXI201  = 6  // overflow P80, um, scale 1
	RegFI202  = 7  // cyclone feed pulp, m3/h, scale 0.1
	RegLI301  = 8  // flotation pulp level, mm, scale 1
	RegFI301  = 9  // aeration air flow, m3/h, scale 1
	RegAI301  = 10 // pulp pH, scale 0.01
	RegQI301  = 11 // collector flow (actual), ml/min, scale 1
	RegQI302  = 12 // frother flow (actual), ml/min, scale 1
	RegDI301  = 13 // flotation pulp solids, %, scale 0.1
	RegAFI301 = 14 // feed Cu grade (XRF), %, scale 0.001
	RegAFC301 = 15 // concentrate Cu grade (XRF), %, scale 0.01
	RegAFT301 = 16 // tails Cu grade (XRF), %, scale 0.001
	RegWI301  = 17 // concentrate dry mass flow, t/h, scale 0.01
	RegWI302  = 18 // tails dry mass flow, t/h, scale 0.1
	RegLI401  = 19 // thickener bed level, m, scale 0.01
	RegDI401  = 20 // underflow solids, %, scale 0.1
	RegEI401  = 21 // rake torque, %, scale 0.1
	RegFI401  = 22 // flocculant dose, g/t, scale 0.1
	RegPI501  = 23 // filter vacuum, kPa, scale 0.1
	RegMI501  = 24 // cake moisture, %, scale 0.01
	RegWI501  = 25 // dry cake throughput, t/h, scale 0.01
	RegTIT101 = 26 // pulp temperature, C, scale 0.1
	RegSI101  = 27 // ore bin level, %, scale 0.1
)

// Holding register offsets (FC3 read, FC6 write).
const (
	RegHC101  = 0 // feeder setpoint, t/h, scale 0.1, write 0..200
	RegFC201  = 1 // mill water valve, %, scale 0.1, write 0..100
	RegFC301  = 2 // collector pump, ml/min, scale 1, write 0..500
	RegFC302  = 3 // frother pump, ml/min, scale 1, write 0..300
	RegLC301  = 4 // tailgate, %, scale 0.1, write 0..100
	RegFC401  = 5 // underflow pump speed, %, scale 0.1, write 0..100
	RegHI501  = 6 // filter cycle time, s, scale 1, write 10..120
	RegSIMCMD = 7 // scenario code, scale 1
	RegSIMVAL = 8 // scenario value, scale 1
)

// regScale is the engineering scale of every register, indexed by offset.
var inputScales = [NumInputRegisters]float64{
	0.1, 0.1, 0.1, 0.1, 0.1, 1, 1, 0.1, // 0..7
	1, 1, 0.01, 1, 1, 0.1, 0.001, 0.01, // 8..15
	0.001, 0.01, 0.1, 0.01, 0.1, 0.1, 0.1, 0.1, // 16..23
	0.01, 0.01, 0.1, 0.1, // 24..27
}

var holdingScales = [NumHoldingRegisters]float64{
	0.1, 0.1, 1, 1, 0.1, 0.1, 1, 1, 1, // 0..8
}

// holdingWriteLimits bounds FC6 writes: [min, max] in raw counts, derived
// from the engineering write ranges of TZ §7.2.
var holdingWriteLimits = [NumHoldingRegisters][2]uint16{
	{0, 2000},   // hc101: 0..200 t/h
	{0, 1000},   // fc201: 0..100 %
	{0, 500},    // fc301: 0..500 ml/min
	{0, 300},    // fc302: 0..300 ml/min
	{0, 1000},   // lc301: 0..100 %
	{0, 1000},   // fc401: 0..100 %
	{100, 1200}, // hi501: 10..120 s
	{0, 255},    // simcmd
	{0, 65535},  // simval (raw i16-like)
}

// Scenario codes written to SIMCMD (TZ §7.3).
const (
	ScenarioNormal     = 0 // reset active disturbances
	ScenarioOreHard    = 1 // step ore hardness by SIMVAL percent
	ScenarioXRFFDrift  = 2 // drift the feed XRF reading on/off (SIMVAL 1/0)
	ScenarioP80Freeze  = 3 // freeze the P80 meter (SIMVAL 1/0)
	ScenarioCommLoss   = 4 // stop answering Modbus (SIMVAL 1/0)
	ScenarioPHDip      = 5 // acid upset: pH dips, recovery takes SIMVAL seconds
	ScenarioUFPumpFail = 6 // underflow pump failure for SIMVAL seconds
	ScenarioBinFill    = 7 // ore bin receives SIMVAL percent
	ScenarioOreType    = 8 // ore type: 0 sulfide, 1 mixed, 2 oxide
)
