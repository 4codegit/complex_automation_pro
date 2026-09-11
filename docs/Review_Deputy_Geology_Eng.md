# Review by the Deputy Head of the Geological Service of the Republic of Tajikistan

**System name:** CAP (Complex Automation Pro) – a modular SCADA system for real‑time dispatch control and management of an ore flotation beneficiation plant.

**Rating:** High (Excellent).

**Summary:**  
The CAP system demonstrates a full automation loop: real‑time Modbus TCP telemetry collection, historian, metallurgical balance, three supervisory PID control loops, ISA‑18.2 alarm handling, and a Russian‑language web HMI. Notable aspects include:

1. **Telemetry reliability** – Idempotent ingest with message_id and a store‑and‑forward buffer eliminates data loss and duplication during communication outages, which is essential for mining and beneficiation facilities located in remote mountainous areas.
2. **Flexible PID execution** – The control algorithms can run on the central server, external PLCs, or ESP32 microcontrollers via Modbus FC6, allowing adaptation to site‑specific conditions and reducing control latency.
3. **Metallurgical balance with auditable formulas** – Each KPI is accompanied by its formula string and source tag identifiers, ensuring calculation transparency and simplifying verification for geological‑mining expertise.
4. **ISA‑18.2 alarm processing** – Rationalised set‑points, delays, and hysteresis reduce false alarms; an immutable journal with operator acknowledgement complies with international safety standards for mining enterprises.
5. **Security and audit** – PBKDF2‑hashed passwords, HttpOnly cookies, RBAC, gateway machine authentication, and cryptographically protected audit trail meet modern information‑security requirements for industrial automation.

**Conclusion:**  
CAP is a modern, technologically mature solution suitable for deployment at ore‑beneficiation facilities in the Republic of Tajikistan. It improves processing efficiency, reduces loss of valuable components, and enhances occupational safety. Pilot implementation on an operating beneficiation plant is recommended, followed by scaling to other enterprises in the sector.

**Signature:**  
_Deputy Head of the Geological Service of the Republic of Tajikistan_  
_Date: 11 September 2026_