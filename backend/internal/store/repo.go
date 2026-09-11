package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrNotFound is returned when a registry row or latest value is absent.
var ErrNotFound = errors.New("not found")

// ErrDuplicate is returned when the (gateway_id, message_id) pair exists.
var ErrDuplicate = errors.New("duplicate message")

// seedAssets is the flotation plant asset tree (TZ §6).
var seedAssets = []Asset{
	{ID: "plant.crushing", Name: "Дробление (бункер, питатель, дробилка)", Area: "Подготовка", Criticality: "high", Active: true},
	{ID: "plant.grinding", Name: "Измельчение (мельница, гидроциклон)", Area: "Подготовка", Criticality: "critical", Active: true},
	{ID: "plant.flotation", Name: "Флотация (rougher/scavenger, реагенты)", Area: "Обогащение", Criticality: "critical", Active: true},
	{ID: "plant.thickening", Name: "Сгущение", Area: "Обезвоживание", Criticality: "high", Active: true},
	{ID: "plant.filtration", Name: "Фильтрация и отгрузка", Area: "Обезвоживание", Criticality: "medium", Active: true},
	{ID: "plant.metallurgy", Name: "Расчётные показатели", Area: "Виртуальный", Criticality: "low", Active: true},
}

// seedTagSpec is one registry row: full tag id suffix, RU label, unit,
// engineering scale and criticality. The prefix ("plant.crushing." etc.) is
// derived from the asset in seedTags below.
type seedTagSpec struct {
	metric   string
	asset    string
	name     string
	unit     string
	min, max float64
	crit     string
}

// seedTags is the flotation plant instrument catalogue (TZ §7): 28 input
// measurements, 7 output actuator positions and 8 derived metallurgical
// values. Register maps live with the edge gateway configuration.
var seedTags = []seedTagSpec{
	// Crushing / ore feed
	{"fi101", "plant.crushing", "Производительность питателя", "t/h", 0, 200, "high"},
	{"ei101", "plant.crushing", "Мощность дробилки", "kW", 0, 400, "medium"},
	{"tit101", "plant.crushing", "Температура пульпы", "C", 5, 45, "low"},
	{"si101", "plant.crushing", "Уровень рудного бункера", "%", 0, 100, "high"},
	{"hc101", "plant.crushing", "Задание питателя", "t/h", 0, 200, "medium"},
	// Grinding / classification
	{"ei201", "plant.grinding", "Мощность мельницы", "kW", 0, 2500, "critical"},
	{"fi201", "plant.grinding", "Свежая вода в мельницу", "m3/h", 0, 300, "medium"},
	{"pi201", "plant.grinding", "Давление питания гидроциклона", "kPa", 0, 350, "high"},
	{"di202", "plant.grinding", "Плотность слива гидроциклона", "g/l", 1300, 1750, "high"},
	{"xi201", "plant.grinding", "Крупность слива P80", "um", 40, 300, "critical"},
	{"fi202", "plant.grinding", "Расход пульпы на гидроциклон", "m3/h", 0, 600, "medium"},
	{"fc201", "plant.grinding", "Клапан воды мельницы", "%", 0, 100, "medium"},
	// Flotation
	{"li301", "plant.flotation", "Уровень пульпы во флотомашине", "mm", 200, 800, "critical"},
	{"fi301", "plant.flotation", "Расход воздуха аэрации", "m3/h", 0, 600, "medium"},
	{"ai301", "plant.flotation", "pH пульпы", "pH", 4, 13, "high"},
	{"qi301", "plant.flotation", "Расход собирателя (факт)", "ml/min", 0, 500, "high"},
	{"qi302", "plant.flotation", "Расход вспенивателя (факт)", "ml/min", 0, 300, "medium"},
	{"di301", "plant.flotation", "Плотность пульпы флотации", "%sol", 10, 45, "high"},
	{"afi301", "plant.flotation", "Содержание Cu в питании", "%Cu", 0.1, 2.0, "high"},
	{"afc301", "plant.flotation", "Содержание Cu в концентрате", "%Cu", 5, 30, "high"},
	{"aft301", "plant.flotation", "Содержание Cu в хвостах", "%Cu", 0.01, 0.5, "critical"},
	{"wi301", "plant.flotation", "Массовый расход концентрата", "t/h", 0, 10, "high"},
	{"wi302", "plant.flotation", "Массовый расход хвостов", "t/h", 0, 200, "medium"},
	{"fc301", "plant.flotation", "Задание насоса собирателя", "ml/min", 0, 500, "medium"},
	{"fc302", "plant.flotation", "Задание насоса вспенивателя", "ml/min", 0, 300, "low"},
	{"lc301", "plant.flotation", "Хвостовая задвижка флотомашины", "%", 0, 100, "critical"},
	// Thickening
	{"li401", "plant.thickening", "Уровень постели сгустителя", "m", 0, 8, "critical"},
	{"di401", "plant.thickening", "Плотность сгущённого продукта", "%sol", 20, 70, "high"},
	{"ei401", "plant.thickening", "Момент гребкового устройства", "%", 0, 100, "critical"},
	{"fi401", "plant.thickening", "Доза флокулянта", "g/t", 0, 50, "medium"},
	{"fc401", "plant.thickening", "Насос разгрузки сгустителя", "%", 0, 100, "high"},
	// Filtration
	{"pi501", "plant.filtration", "Вакуум фильтра", "kPa", 0, 80, "high"},
	{"mi501", "plant.filtration", "Влажность кека", "%", 4, 25, "high"},
	{"wi501", "plant.filtration", "Производительность по сухому кеку", "t/h", 0, 10, "medium"},
	{"hi501", "plant.filtration", "Время цикла фильтра", "s", 10, 120, "low"},
	// Derived metallurgical values (virtual, written by the calc service)
	{"calc_epsilon", "plant.metallurgy", "Извлечение Cu", "%", 0, 100, "critical"},
	{"calc_gamma", "plant.metallurgy", "Выход концентрата", "%", 0, 20, "high"},
	{"calc_upgrade", "plant.metallurgy", "Коэффициент обогащения", "x", 0, 50, "medium"},
	{"calc_pull", "plant.metallurgy", "Съём концентрата", "%", 0, 20, "medium"},
	{"calc_balance_err", "plant.metallurgy", "Невязка массового баланса", "%", -10, 10, "high"},
	{"calc_q_collector", "plant.metallurgy", "Удельный расход собирателя", "ml/t", 0, 500, "medium"},
	{"calc_bond_kwt", "plant.metallurgy", "Удельная энергия измельчения (Бонд)", "kWh/t", 0, 30, "medium"},
	{"calc_cl_pct", "plant.metallurgy", "Циркулирующая нагрузка", "%", 0, 600, "medium"},
}

// SeedRegistry inserts the flotation plant asset/tag catalogue when the tables
// are empty (TZ §6/§7). Live plants populate the registry through the
// change-controlled administrative workflow instead.
func SeedRegistry(ctx context.Context, db *sql.DB) error {
	var n int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM assets`).Scan(&n); err != nil {
		return fmt.Errorf("count assets: %w", err)
	}
	if n > 0 {
		return nil
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, a := range seedAssets {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO assets (id, name, area, criticality, active) VALUES (?, ?, ?, ?, ?)`,
			a.ID, a.Name, a.Area, a.Criticality, boolToInt(a.Active)); err != nil {
			return fmt.Errorf("seed asset %s: %w", a.ID, err)
		}
	}
	for _, spec := range seedTags {
		direction := DirectionInput
		if spec.metric == "hc101" || spec.metric == "fc201" || spec.metric == "fc301" ||
			spec.metric == "fc302" || spec.metric == "lc301" || spec.metric == "fc401" ||
			spec.metric == "hi501" {
			direction = DirectionOutput
		}
		t := Tag{
			ID:                      spec.asset + "." + spec.metric,
			AssetID:                 spec.asset,
			Name:                    spec.name,
			Unit:                    spec.unit,
			DataType:                "number",
			SamplingIntervalSeconds: 1.0,
			Criticality:             spec.crit,
			Active:                  true,
			Direction:               direction,
		}
		t.EngineeringMin, t.EngineeringMax = &spec.min, &spec.max
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO tags (id, asset_id, name, unit, data_type, engineering_min, engineering_max,
			  sampling_interval_seconds, criticality, active, direction)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			t.ID, t.AssetID, t.Name, t.Unit, t.DataType, t.EngineeringMin, t.EngineeringMax,
			t.SamplingIntervalSeconds, t.Criticality, boolToInt(t.Active), t.Direction); err != nil {
			return fmt.Errorf("seed tag %s: %w", t.ID, err)
		}
	}
	return tx.Commit()
}

// ListAssets returns the equipment tree ordered by area and name.
func ListAssets(ctx context.Context, db *sql.DB) ([]Asset, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, name, area, criticality, active FROM assets ORDER BY area, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Asset, 0)
	for rows.Next() {
		var a Asset
		var active int
		if err := rows.Scan(&a.ID, &a.Name, &a.Area, &a.Criticality, &active); err != nil {
			return nil, err
		}
		a.Active = active == 1
		out = append(out, a)
	}
	return out, rows.Err()
}

// ListTags returns registered tags, optionally filtered by asset and/or direction.
func ListTags(ctx context.Context, db *sql.DB, assetID string) ([]Tag, error) {
	q := `SELECT id, asset_id, name, unit, data_type, engineering_min, engineering_max,
		sampling_interval_seconds, criticality, active, direction FROM tags`
	var args []any
	if assetID != "" {
		q += ` WHERE asset_id = ?`
		args = append(args, assetID)
	}
	q += ` ORDER BY asset_id, id`

	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Tag, 0)
	for rows.Next() {
		var t Tag
		var active int
		if err := rows.Scan(&t.ID, &t.AssetID, &t.Name, &t.Unit, &t.DataType,
			&t.EngineeringMin, &t.EngineeringMax, &t.SamplingIntervalSeconds,
			&t.Criticality, &active, &t.Direction); err != nil {
			return nil, err
		}
		t.Active = active == 1
		out = append(out, t)
	}
	return out, rows.Err()
}

// GetTag returns a single tag by id, or ErrNotFound.
func GetTag(ctx context.Context, db *sql.DB, id string) (*Tag, error) {
	var t Tag
	var active int
	err := db.QueryRowContext(ctx,
		`SELECT id, asset_id, name, unit, data_type, engineering_min, engineering_max,
			sampling_interval_seconds, criticality, active, direction FROM tags WHERE id = ?`, id).
		Scan(&t.ID, &t.AssetID, &t.Name, &t.Unit, &t.DataType,
			&t.EngineeringMin, &t.EngineeringMax, &t.SamplingIntervalSeconds,
			&t.Criticality, &active, &t.Direction)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	t.Active = active == 1
	return &t, nil
}

// InsertReading stores a reading if its (gateway_id, message_id) is new.
// It returns ErrDuplicate when the pair already exists. The single-statement
// upsert keeps the write lock held briefly, which matters at the ingest rate
// (an explicit BEGIN/SELECT/INSERT/COMMIT transaction serialises writers and
// caused SQLITE_BUSY bursts under load).
func InsertReading(ctx context.Context, db *sql.DB, r *Reading) error {
	res, err := db.ExecContext(ctx,
		`INSERT INTO telemetry_readings
		 (id, message_id, gateway_id, source_sequence, observed_at, received_at, sent_at,
		  asset_id, tag_id, value_number, value_bool, value_string, value_structured,
		  unit, quality, profile_id)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT (gateway_id, message_id) DO NOTHING`,
		r.ID, r.MessageID, r.GatewayID, r.SourceSequence,
		FormatUTC(r.ObservedAt), FormatUTC(r.ReceivedAt), nullTime(r.SentAt),
		r.AssetID, r.TagID, r.ValueNumber, r.ValueBool, r.ValueString, r.ValueStructured,
		r.Unit, r.Quality, r.ProfileID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrDuplicate
	}
	return nil
}

// ReadingFilter narrows a history query.
type ReadingFilter struct {
	TagID   string
	AssetID string
	From    *time.Time
	To      *time.Time
	Limit   int
}

// QueryReadings returns readings in descending observed_at order.
func QueryReadings(ctx context.Context, db *sql.DB, f ReadingFilter) ([]Reading, error) {
	q := `SELECT id, message_id, gateway_id, source_sequence, observed_at, received_at, sent_at,
		asset_id, tag_id, value_number, value_bool, value_string, value_structured,
		unit, quality, profile_id FROM telemetry_readings`
	var conds []string
	var args []any
	if f.TagID != "" {
		conds = append(conds, "tag_id = ?")
		args = append(args, f.TagID)
	}
	if f.AssetID != "" {
		conds = append(conds, "asset_id = ?")
		args = append(args, f.AssetID)
	}
	if f.From != nil {
		conds = append(conds, "observed_at >= ?")
		args = append(args, FormatUTC(*f.From))
	}
	if f.To != nil {
		conds = append(conds, "observed_at <= ?")
		args = append(args, FormatUTC(*f.To))
	}
	if len(conds) > 0 {
		q += " WHERE " + strings.Join(conds, " AND ")
	}
	q += " ORDER BY observed_at DESC LIMIT ?"
	args = append(args, f.Limit)

	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanReadings(rows)
}

// LatestReading returns the newest reading for a tag, or ErrNotFound.
func LatestReading(ctx context.Context, db *sql.DB, tagID string) (*Reading, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT id, message_id, gateway_id, source_sequence, observed_at, received_at, sent_at,
			asset_id, tag_id, value_number, value_bool, value_string, value_structured,
			unit, quality, profile_id FROM telemetry_readings
		 WHERE tag_id = ? ORDER BY observed_at DESC LIMIT 1`, tagID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	rs, err := scanReadings(rows)
	if err != nil {
		return nil, err
	}
	if len(rs) == 0 {
		return nil, ErrNotFound
	}
	return &rs[0], nil
}

func scanReadings(rows *sql.Rows) ([]Reading, error) {
	var out []Reading
	for rows.Next() {
		var r Reading
		var obs, rec string
		var sent sql.NullString
		if err := rows.Scan(&r.ID, &r.MessageID, &r.GatewayID, &r.SourceSequence,
			&obs, &rec, &sent, &r.AssetID, &r.TagID,
			&r.ValueNumber, &r.ValueBool, &r.ValueString, &r.ValueStructured,
			&r.Unit, &r.Quality, &r.ProfileID); err != nil {
			return nil, err
		}
		t, err := time.Parse(time.RFC3339Nano, obs)
		if err != nil {
			return nil, fmt.Errorf("parse observed_at %q: %w", obs, err)
		}
		r.ObservedAt = t
		t, err = time.Parse(time.RFC3339Nano, rec)
		if err != nil {
			return nil, fmt.Errorf("parse received_at %q: %w", rec, err)
		}
		r.ReceivedAt = t
		if sent.Valid {
			st, err := time.Parse(time.RFC3339Nano, sent.String)
			if err != nil {
				return nil, fmt.Errorf("parse sent_at %q: %w", sent.String, err)
			}
			r.SentAt = &st
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// InsertAlert persists an alert row.
func InsertAlert(ctx context.Context, db *sql.DB, a *Alert) error {
	_, err := db.ExecContext(ctx,
		`INSERT INTO alerts (id, stage, metric, value, threshold, message, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		a.ID, a.Stage, a.Metric, a.Value, a.Threshold, a.Message, FormatUTC(a.CreatedAt))
	return err
}

// ListAlerts returns alerts newest-first with optional stage/metric filters.
func ListAlerts(ctx context.Context, db *sql.DB, stage, metric string, limit int) ([]Alert, error) {
	q := `SELECT id, stage, metric, value, threshold, message, created_at FROM alerts`
	var conds []string
	var args []any
	if stage != "" {
		conds = append(conds, "stage = ?")
		args = append(args, stage)
	}
	if metric != "" {
		conds = append(conds, "metric = ?")
		args = append(args, metric)
	}
	if len(conds) > 0 {
		q += " WHERE " + strings.Join(conds, " AND ")
	}
	q += " ORDER BY created_at DESC LIMIT ?"
	args = append(args, limit)

	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Alert, 0)
	for rows.Next() {
		var a Alert
		var created string
		if err := rows.Scan(&a.ID, &a.Stage, &a.Metric, &a.Value, &a.Threshold, &a.Message, &created); err != nil {
			return nil, err
		}
		t, err := time.Parse(time.RFC3339Nano, created)
		if err != nil {
			return nil, fmt.Errorf("parse created_at %q: %w", created, err)
		}
		a.CreatedAt = t
		out = append(out, a)
	}
	return out, rows.Err()
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func nullTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return FormatUTC(*t)
}

// CreateAsset inserts a new asset. Returns an error on duplicate ID.
func CreateAsset(ctx context.Context, db *sql.DB, a *Asset) error {
	a.ID = strings.TrimSpace(a.ID)
	if a.ID == "" {
		a.ID = NewID()
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO assets (id, name, area, criticality, active) VALUES (?, ?, ?, ?, ?)`,
		a.ID, a.Name, a.Area, a.Criticality, boolToInt(a.Active)); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "unique") {
			return fmt.Errorf("asset %q already exists", a.ID)
		}
		return err
	}
	return nil
}

// UpdateAsset overwrites name, area, criticality and active for an existing asset.
func UpdateAsset(ctx context.Context, db *sql.DB, a *Asset) error {
	res, err := db.ExecContext(ctx,
		`UPDATE assets SET name=?, area=?, criticality=?, active=? WHERE id=?`,
		a.Name, a.Area, a.Criticality, boolToInt(a.Active), a.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteAsset removes an asset if no tags reference it.
func DeleteAsset(ctx context.Context, db *sql.DB, id string) error {
	var cnt int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM tags WHERE asset_id=?`, id).Scan(&cnt); err != nil {
		return err
	}
	if cnt > 0 {
		return fmt.Errorf("cannot delete asset %q: %d tag(s) still reference it", id, cnt)
	}
	res, err := db.ExecContext(ctx, `DELETE FROM assets WHERE id=?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// CreateTag inserts a new tag. Returns an error on duplicate ID.
func CreateTag(ctx context.Context, db *sql.DB, t *Tag) error {
	t.ID = strings.TrimSpace(t.ID)
	if t.ID == "" {
		t.ID = NewID()
	}
	// Verify the referenced asset exists.
	var a string
	if err := db.QueryRowContext(ctx, `SELECT id FROM assets WHERE id=?`, t.AssetID).Scan(&a); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("asset %q does not exist", t.AssetID)
		}
		return err
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO tags (id, asset_id, name, unit, data_type, engineering_min, engineering_max,
		 sampling_interval_seconds, criticality, active, direction)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.AssetID, t.Name, t.Unit, t.DataType,
		t.EngineeringMin, t.EngineeringMax, t.SamplingIntervalSeconds,
		t.Criticality, boolToInt(t.Active), t.Direction); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "unique") {
			return fmt.Errorf("tag %q already exists", t.ID)
		}
		return err
	}
	return nil
}

// UpdateTag overwrites mutable fields for an existing tag.
func UpdateTag(ctx context.Context, db *sql.DB, t *Tag) error {
	res, err := db.ExecContext(ctx,
		`UPDATE tags SET asset_id=?, name=?, unit=?, data_type=?,
		 engineering_min=?, engineering_max=?, sampling_interval_seconds=?,
		 criticality=?, active=?, direction=? WHERE id=?`,
		t.AssetID, t.Name, t.Unit, t.DataType,
		t.EngineeringMin, t.EngineeringMax, t.SamplingIntervalSeconds,
		t.Criticality, boolToInt(t.Active), t.Direction, t.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteTag removes a tag if no telemetry readings reference it.
func DeleteTag(ctx context.Context, db *sql.DB, id string) error {
	var cnt int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM telemetry_readings WHERE tag_id=?`, id).Scan(&cnt); err != nil {
		return err
	}
	if cnt > 0 {
		return fmt.Errorf("tag %q has %d historical readings; cannot delete", id, cnt)
	}
	res, err := db.ExecContext(ctx, `DELETE FROM tags WHERE id=?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
