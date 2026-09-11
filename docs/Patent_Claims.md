# Patent Claims for CAP (Complex Automation Pro)

## Independent Claims

1. **System Architecture Claim**  
   A distributed automation system for an ore flotation beneficiation plant comprising:  
   (a) a single-server modular monolith (capd) providing a REST API, WebSocket interface, authentication, role‑based access control, telemetry ingest, historian, alarm management (ISA‑18.2), supervisory PID control loops, metallurgical calculations, and an embedded single‑page web‑application;  
   (b) a dedicated gateway process (edge) that polls a Modbus TCP source at a configurable interval, normalises received values to a unified telemetry contract, buffers transmissions via a store‑and‑forward mechanism with message‑id based exactly‑once delivery, and writes actuator setpoints to the source using function code 6 (FC6) exclusively when the associated supervisory PID loop is in automatic mode;  
   (c) a process simulator (plantsim) that emulates the plant via a real Modbus TCP server (FC3/FC4/FC6) and provides an HTTP interface for scenario injection;  
   wherein the sole path of telemetry from the physical plant (or simulator) to the monolith is: plant → Modbus TCP → edge → ingest (idempotent) → capd → historian/WebSocket, and virtual calculated tags (calc_*) traverse the same ingest contract.

2. **Telemetry Ingest Claim**  
   The system of claim 1, wherein the ingest service in capd assigns a monotonically increasing message_id to each telemetry batch, persists received batches in a SQLite WAL database, acknowledges receipt to the edge only after durable storage, and upon reconnection retransmits any unacknowledged batches preserving original order and preventing duplicate insertion into the historian.

3. **Flexible PID Execution Claim**  
   The system of claim 1, wherein the supervisory PID control algorithms for regulating plant variables (level, reagent flow, density) are configurable to execute in any of the following execution contexts without altering the telemetry contract:  
   (i) within the capd monolith as a native controlsvc module;  
   (ii) on an external programmable logic controller (PLC) communicating with capd via Modbus function codes 3/4/6;  
   (iii) on an ESP32‑class microcontroller implementing a Modbus slave and receiving setpoint/process value through FC3/FC4 and delivering control output via FC6;  
   and wherein the edge forwards PID‑computed output to the plant only when the corresponding loop is in automatic mode, otherwise preserving the last valid output.

4. **Adaptive PID Tuning Claim**  
   The system of claim 3, further comprising a gain‑scheduling module that adjusts the proportional (Kp) and integral (Ki) coefficients of each PID loop in real time based on a measurable process gain indicator (e.g., feeder flow rate), wherein the module updates the coefficients without requiring loop re‑initialisation and includes anti‑windup protection that limits the integral term when the actuator output reaches its configured limits.

5. **Automatic pH Regulation Claim**  
   The system of claim 1, wherein a dedicated PID loop controls the pH of the flotation pulp by manipulating the dosage of acid or alkali reagents, the loop operates fully automatically (no operator intervention required) and includes:  
   (a) a dead‑band of ±0.2 pH units around the setpoint that generates a warning alarm, and a wider dead‑band of ±0.5 pH units that generates a critical alarm and forces the loop into manual mode with output frozen;  
   (b) self‑tuning of Kp/Ki based on pulp temperature and flow rate;  
   (c) bumpless transfer from manual to automatic mode by initializing the integral term from the current actuator position.

6. **ISA‑18.2 Alarm Processing Claim**  
   The system of claim 1, wherein alarm evaluation is performed on each incoming telemetry sample in the capd monolith, employing:  
   (a) statistically derived low‑low/low/high/high‑high limits using a moving window standard deviation;  
   (b) a hysteresis of 1 % of the alarm scale to prevent chattering;  
   (c) configurable delay timers: 3 seconds for critical alarms, 10 seconds for medium/low alarms;  
   (d) automatic assignment of alarm priority based on the impact of the associated variable on product quality or equipment safety;  
   (e) an immutable alarm event log storing timestamp, tag identifier, alarm state (active/returned/acknowledged), and the operator identifier from the active session;  
   (f) acknowledgement functionality that requires an explicit operator comment and records the acknowledging operator and timestamp.

7. **Secure Audit and Gateway Authentication Claim**  
   The system of claim 1, further comprising:  
   (a) a cryptographically protected audit trail in which each mutating API request (including setpoint changes, alarm acknowledgements, scenario triggers) is signed with a session‑specific key and a timestamp, and the signature is stored alongside the request payload in an append‑only log;  
   (b) gateway authentication wherein each edge process proves possession of a shared secret `GATEWAY_TOKEN` by computing a constant‑time HMAC‑SHA256 over a nonce supplied by capd during the ingest handshake;  
   (c) role‑based access control that distinguishes operator privileges (alarm acknowledgement, setpoint modification, scenario launch) from administrator privileges (user/role management, tag registry, audit log access).

## Dependent Claims (examples)

8. The system of claim 2, wherein the SQLite database operates in WAL mode with a synchronous setting of NORMAL to balance durability and throughput.

9. The system of claim 3, wherein the external PLC or ESP32 implements a watchdog that forces its PID output to the last safe value upon loss of communication with capd for more than a configurable timeout.

10. The system of claim 5, wherein the acid/alkali reagent dosing actuators are configured with output limits that correspond to the mechanical limits of the dosing pumps, and the anti‑windup module integrates the difference between the demanded and actual output to prevent integral accumulation during saturation.

11. The system of claim 6, wherein the moving window used for statistical limit calculation is configurable between 50 and 500 samples, and the standard deviation is updated using Welford’s online algorithm to minimise numerical error.

12. The system of claim 7(a), wherein the audit signature is generated using an Ed25519 private key derived from the session’s authentication token, enabling efficient verification without exposing the private key.

13. The system of claim 7(b), wherein the constant‑time comparison of the HMAC prevents timing‑side‑channel attacks on the gateway authentication process.

14. The system of claim 1, wherein the web‑application is built with React 18, TypeScript, Vite, and Tailwind CSS, and is served as static assets from the capd monolith, enabling operation in an air‑gapped environment without external CDN dependencies.

15. The system of claim 1, wherein the metallurgical calculations include: two‑product mass balance, copper recovery ε = β(α−θ)/(α(β−θ))×100, concentrate output γ, enrichment ratio K = β/α, mass pull, balance error, energy per Bond, mill efficiency, circulating load, and specific reagent consumption, each calculation being accompanied by its explicit formula string and the list of source tag identifiers for auditability.