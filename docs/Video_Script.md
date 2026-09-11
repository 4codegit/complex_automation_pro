# Video Script – CAP Demonstration (2‑3 minutes)

**Target length:** 150 seconds  
**Style:** Narrated walkthrough with on‑screen callouts, background music low, occasional sound effects for clicks/alarms.  
**Resolution:** 1920×1080, 30 fps.  
**Narration:** Clear, confident, Russian language (can be subtitled in English).  

---

### 0:00‑0:05 – Opening Shot
- **Visual:** Logo “CAP – Complex Automation Pro” fades in over a dark background, subtle animation of data flowing into a central node.
- **Narration:**  
  «CAP – модульная SCADA‑система для оперативного диспетчерского контроля и управления участком флотационного обогащения руды. За 150 секунд мы покажем, как она обеспечивает полный контур автоматизации, надёжную телеметрию и интеллектуальное управление без постоянного вмешательства оператора.»

### 0:05‑0:15 – System Architecture Diagram
- **Visual:** Animated block diagram (as in README) – capd, edge, plantsim, with arrows showing Modbus TCP, HTTP batch, FC6.
- **On‑screen callout:**  
  «Единственный путь данных: станция → Modbus → edge → ingest → capd → историк/WebSocket. Виртуальные теги calc_* проходят тот же контракт.»
- **Narration:**  
  «Архитектура построена вокруг модульного монолита capd, шлюза сбора edge и симулятора plantsim. Все данные проходят единый контракт ingest с идемпотентной доставкой.»

### 0:15‑0:25 – Launch Sequence (backend/run.sh)
- **Visual:** Terminal window showing `cd backend && ./run.sh`, then output lines: building, starting plantsim, capd, edge, finally “Готово: http://127.0.0.1:8000”.
- **Narration:**  
  «Запуск выполняется одной командой. Скрипт собирает три процесса, создаёт чистую базу данных и открывает веб‑интерфейс.»

### 0:25‑0:35 – Login Screen
- **Visual:** Browser opens to `http://127.0.0.1:8000`, login form appears. Credentials `operator/operator` entered, submit.
- **On‑screen callout:**  
  «Локальные учётные записи, PBKDF2‑SHA256, HttpOnly‑сессии.»
- **Narration:**  
  «Вход в систему – оператор или администратор. Пароли – демо‑seed, подлежат замене в production.»

### 0:35‑0:50 – Overview Page (Mimic Scheme)
- **Visual:** Main page loads – left navigation, central мнимосхема (synoptic‑light.png). Nodes coloured green (normal). Hover over a node shows tooltip with tag name and value.
- **Narration:**  
  «Главная страница – вертикальная технологическая схема. Цвет узла отражает состояние: норма, тревога, нет связи. Клик по узлу открывает паспорт объекта (faceplate).»

### 0:50‑1:05 – Faceplate (Object Passport)
- **Visual:** Click on flotation node → modal window with tables: measured values (good/offline), PID panels (PV, SP, MV), alarm settings.
- **On‑screen callout:**  
  «Паспорт объекта: все параметры с качеством, панели ПИД‑контуров, кнопки с подтверждением.»
- **Narration:**  
  «Из паспорта можно сразу менять уставки контуров, переводить их в автомат или ручной режим, видеть текущие значения и состояние тревог.»

### 1:05‑1:20 – Trend Page
- **Visual:** Navigate to “Тренды”. Add several tags (e.g., li301, fi301, di401, epsilon). Set range 15 min. Show live updating graphs.
- **On‑screen callout:**  
  «Тренды с таблицей перьев, диапазон 15 мин‑24 ч, нормировка. Разрыв линии = отсутствие достоверных данных (offline).»
- **Narration:**  
  «Страница трендов позволяет выбирать любые теги, включая виртуальные рассчётные показатели, и наблюдать их динамику в реальном времени.»

### 1:20‑1:35 – Metallurgy Page
- **Visual:** Switch to “Металлургия”. Show the balance summary with KPI cards: ε (recovery), γ (output), K (enrichment), balance error, Bond energy, circulating load, specific collector consumption. Each card displays formula string and source tags.
- **On‑screen callout:**  
  «Каждый KPI содержит строку формулы и теги‑источники – аудируемость расчёта. Индикатор «баланс сходится» зелёный при невязке ≤±1 %.»
- **Narration:**  
  «Металлургический блок рассчитывает двухпродуктовый баланс, извлечение, выход, коэффициент обогащения, энергию Бонда и другие показатели, каждая формула сопровождается указанием источников данных.»

### 1:35‑1:50 – Control Page – Switching to Automatic
- **Visual:** Open “Управление”. Show three PID loops: lic301 (level), fic301 (collector flow), dic401 (density).  
  - Click lic301 mode button → switch from MANUAL to AUTO, confirm popup appears, user clicks “Подтвердить”.  
  - Then edit SP field: change from 500 to 550, click “Изменить SP”, confirm.
- **On‑screen callout:**  
  «Режимы АВТО/РУЧН, запись уставок и подтверждение в журнал аудита. Безбамперный переход: интеграл инициализируется от фактической позиции актуатора.»
- **Narration:**  
  «Перевод контура в автомат и изменение уставки выполняются только с явным подтверждением оператора, после чего изменение фиксируется в аудит. При переходе в АВТО интеграл инициализируется от текущей позиции, исключая скачки.»

### 1:50‑2:05 – Watchdog Simulation
- **Visual:** Simulate loss of PV: in the trend for li301, the line goes flat (offline). After ~10 seconds, the lic301 loop icon changes to MANUAL, output value freezes, a red alarm banner appears at top: “control_fault – lic301”.
- **On‑screen callout:**  
  «Watchdog 10 с: при потере достоверного PV контур переходит в РУЧН, выход замораживается, формируется тревога control_fault (IEC 61511).»
- **Narration:**  
  «Если процессная переменная становится недоступной более 10 секунд, watchdog автоматически переводит контур в ручной режим, замораживая выход – это обеспечивает безопасное состояние по стандарту IEC 61511.»

### 2:05‑2:20 – Alarm Page – ISA‑18.2 Processing
- **Visual:** Navigate to “Тревоги”. Show list of active alarms: lic301 control_fault (critical), maybe a warning for pH if we had changed it earlier. Show journal tab with entries: “сработала”, “сброшена”, “квитирована” with timestamps and operator name.
- **On‑screen callout:**  
  «Тревоги по ISA‑18.2: рационализированные уставки, задержки 3/10 с, гистерезис 1 %, неизменяемый журнал, квитирование с комментарием оператора.»
- **Narration:**  
  «Страница тревог отображает активные события и журнал. Тревоги оцениваются с задержками и гистерезисом, журнал неизменяем, а квитирование требует обязательного комментария от оператора.»

### 2:20‑2:35 – Store‑and‑Forward Demo (Edge Buffer)
- **Visual:** Switch back to terminal showing edge logs. Simulate network loss: disconnect the virtual link between edge and capd (e.g., stop capd). Show edge log: “store‑and‑forward: buffering…”. After a few seconds, restart capd. Edge log: “reconnecting, sending buffered batches…”. Show capd historian receiving the backlog.
- **On‑screen callout:**  
  «Буфер SQLite store‑and‑forward: при обрыве связь edge→capd данные сохраняются, после восстановления передаются exactamente‑once без дублей.»
- **Narration:**  
  «Шлюз использует store‑and‑forward буфер: при потере связи с сервером телеметрия накапливается локально и после восстановления отправляется без потерь и дублей.»

### 2:35‑2:50 – Security & Audit Snapshot
- **Visual:** Open “Администрирование” → “Доступ и аудит”. Show a table of recent mutations: user, timestamp, action (e.g., “set SP lic301 = 550”), signature column showing a hash.
- **On‑screen callout:**  
  «Криптографически защищённый аудит: каждая мутация подписана сессией и меткой времени. Шлюзы аутентифицируются общим секретом постоянного времени сравнения.»
- **Narration:**  
  «Раздел аудита фиксирует каждое изменение с информацией о пользователе и времени, а также криптографической подписью. Шлюзы проходят машинную аутентификацию по общему токену.»

### 2:50‑3:00 – Closing & Call to Action
- **Visual:** Fade back to CAP logo, with bullet points appearing:  
  • Полный контур автоматизации (реальный Modbus → edge → capd → FC6)  
  • Металлургический баланс с аудируемыми формулами  
  • Гибкие ПИД‑контуры (capd/PLC/ESP32)  
  • Автоматическое регулирование pH  
  • Тревоги ISA‑18.2 с неизменяемым журналом  
  • Надёжная телеметрия и защищённый аудит  
- **Narration:**  
  «CAP демонстрирует, что даже в минимальном жизнеспособном продукте можно достичь уровня функциональности и надёжности, сравнимого с промышленными SCADA‑решениями, при этом сохраняя простоту внедрения, низкую стоимость и открытость архитектуры для будущего расширения. Спасибо за внимание.»  
- **Visual:** Fade to black, show contact info or website placeholder.

---  
**End of Script**