package main

import "time"

// ---------------------------------------------------------------------------
// Modèles — alignés sur les interfaces de server.ts et src/types/index.ts.
// ---------------------------------------------------------------------------

type Plan struct {
	ID                 string         `json:"id"`
	Code               string         `json:"code"`
	Name               string         `json:"name"`
	Description        string         `json:"description"`
	PriceXOF           int            `json:"price_xof"`
	BillingPeriod      string         `json:"billing_period"`
	MaxStations        int            `json:"max_stations"`
	MaxEmployees       int            `json:"max_employees"`
	MaxWashOrdersMonth int            `json:"max_wash_orders_month"`
	TrialDays          int            `json:"trial_days"`
	Features           map[string]any `json:"features"`
	SortOrder          int            `json:"sort_order"`
	IsActive           bool           `json:"is_active"`
}

type Organization struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	Email     string    `json:"email"`
	Phone     string    `json:"phone"`
	Address   string    `json:"address"`
	City      string    `json:"city"`
	Country   string    `json:"country"`
	Currency  string    `json:"currency"`
	Timezone  string    `json:"timezone"`
	LogoURL   string    `json:"logo_url"`
	Status    string    `json:"status"`
	Notes     string    `json:"notes,omitempty"`
	CreatedAt time.Time `json:"created_at"`

	// Identité commerciale (facultative : un compte neuf n'a que son nom).
	LegalName           string `json:"legal_name"`
	TradeName           string `json:"trade_name"`
	ActivityType        string `json:"activity_type"`
	ActivityDescription string `json:"activity_description"`
	// Date d'ouverture, au format « 2006-01-02 », vide si non renseignée
	// (colonne DATE, consommée telle quelle par l'interface).
	ActivityStartedOn string `json:"activity_started_on"`
	WhatsApp          string `json:"whatsapp"`
	Website           string `json:"website"`

	// Facturation : à qui adresser la facture, si c'est différent du siège.
	BillingName    string `json:"billing_name"`
	BillingAddress string `json:"billing_address"`
	BillingEmail   string `json:"billing_email"`
	BillingPhone   string `json:"billing_phone"`

	// Mentions administratives : la valeur est ici, son état de vérification
	// vit dans `organization_registry_fields` (voir org_profile.go).
	LegalForm           string `json:"legal_form"`
	LegalRepresentative string `json:"legal_representative"`
	RCCM                string `json:"rccm"`
	TaxID               string `json:"tax_id"`
	CNPSEmployerNumber  string `json:"cnps_employer_number"`
	FiscalRegime        string `json:"fiscal_regime"`
	LegalMentions       string `json:"legal_mentions"`
}

type Station struct {
	ID             string         `json:"id"`
	OrganizationID string         `json:"organization_id"`
	Name           string         `json:"name"`
	Code           string         `json:"code"`
	Phone          string         `json:"phone"`
	Address        string         `json:"address"`
	City           string         `json:"city"`
	Timezone       string         `json:"timezone"`
	Currency       string         `json:"currency"`
	LogoURL        string         `json:"logo_url"`
	OpeningHours   map[string]any `json:"opening_hours"`
	Settings       map[string]any `json:"settings"`
	IsDefault      bool           `json:"is_default"`
	IsActive       bool           `json:"is_active"`

	// Localisation : le pays, la ville et le quartier servent à retrouver la
	// station, les coordonnées à l'afficher sur une carte (facultatives).
	Country        string    `json:"country"`
	District       string    `json:"district"`
	Commune        string    `json:"commune"`
	DistrictArea   string    `json:"district_area"`
	Street         string    `json:"street"`
	Landmark       string    `json:"landmark"`
	Latitude       string    `json:"latitude"`
	Longitude      string    `json:"longitude"`
	BillingAddress string    `json:"billing_address"`
	CreatedAt      time.Time `json:"created_at"`
}

type Subscription struct {
	ID                 string     `json:"id"`
	OrganizationID     string     `json:"organization_id"`
	PlanID             string     `json:"plan_id"`
	Status             string     `json:"status"`
	StartedAt          time.Time  `json:"started_at"`
	TrialEndsAt        *time.Time `json:"trial_ends_at"`
	CurrentPeriodStart *time.Time `json:"current_period_start"`
	CurrentPeriodEnd   *time.Time `json:"current_period_end"`
	Notes              string     `json:"notes"`
}

type User struct {
	ID                   string     `json:"id"`
	OrganizationID       *string    `json:"organization_id"`
	StationID            *string    `json:"station_id"`
	FullName             string     `json:"full_name"`
	Email                string     `json:"email"`
	Phone                string     `json:"phone"`
	PasswordHash         string     `json:"-"`
	Role                 string     `json:"role"`
	IsActive             bool       `json:"is_active"`
	MustChangePassword   bool       `json:"must_change_password"`
	LastLoginAt          *time.Time `json:"last_login_at"`
	CreatedAt            time.Time  `json:"created_at"`
	CommissionMode       *string    `json:"commission_mode"`
	CommissionPerWashXOF *int       `json:"commission_per_wash_xof"`
	CommissionPercent    *float64   `json:"commission_percent"`
}

type VehicleCategory struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	SortOrder   int    `json:"sort_order"`
	IsActive    bool   `json:"is_active"`
}

type Service struct {
	ID              string `json:"id"`
	OrganizationID  string `json:"organization_id"`
	StationID       string `json:"station_id"`
	Name            string `json:"name"`
	Category        string `json:"category"`
	Description     string `json:"description"`
	PriceXOF        int    `json:"price_xof"`
	DurationMinutes int    `json:"duration_minutes"`
	SortOrder       int    `json:"sort_order"`
	IsActive        bool   `json:"is_active"`
	// Depuis le lot 5 : famille, code court, contenu détaillé et consignes.
	Code           string   `json:"code"`
	FamilyCode     string   `json:"family_code"`
	FamilyName     string   `json:"family_name"`
	TemplateCode   string   `json:"template_code"`
	Includes       []string `json:"includes"`
	EquipmentNotes string   `json:"equipment_notes"`
	SafetyNotes    string   `json:"safety_notes"`
	Preconditions  string   `json:"preconditions"`
	// « Gratuit » est une décision : une prestation non tarifée reste à
	// price_xof = 0 sans is_free, et le serveur refuse de la vendre.
	IsFree      bool `json:"is_free"`
	IsQuoteOnly bool `json:"is_quote_only"`
	// Vrai quand aucun tarif utilisable n'existe : l'écran affiche « à tarifer ».
	NeedsPrice bool `json:"needs_price"`
}

type Customer struct {
	ID             string     `json:"id"`
	OrganizationID string     `json:"organization_id"`
	StationID      string     `json:"station_id"`
	FullName       string     `json:"full_name"`
	Phone          string     `json:"phone"`
	Plate          string     `json:"plate"`
	VehicleModel   string     `json:"vehicle_model"`
	Notes          string     `json:"notes"`
	IsActive       bool       `json:"is_active"`
	CreatedAt      time.Time  `json:"created_at"`
	WashCount      int        `json:"wash_count"`
	TotalSpentXOF  int        `json:"total_spent_xof"`
	LastVisitAt    *time.Time `json:"last_visit_at"`
}

type WashOrderItem struct {
	ID           string `json:"id"`
	WashOrderID  string `json:"wash_order_id"`
	ServiceID    string `json:"service_id"`
	ServiceName  string `json:"service_name"`
	UnitPriceXOF int    `json:"unit_price_xof"`
	Quantity     int    `json:"quantity"`
	LineTotalXOF int    `json:"line_total_xof"`
	// D'où vient le prix appliqué (`client_contract`, `service_category`…) :
	// une ligne ancienne doit pouvoir s'expliquer sans rejouer la grille.
	PriceSource      string `json:"price_source"`
	PriceSourceLabel string `json:"price_source_label"`
	// Options retenues au moment de la vente, avec leur prix d'alors.
	Options         []optionRetenue `json:"options"`
	OptionsTotalXOF int             `json:"options_total_xof"`
}

type WashOrder struct {
	ID                   string     `json:"id"`
	OrganizationID       string     `json:"organization_id"`
	StationID            string     `json:"station_id"`
	Reference            string     `json:"reference"`
	CustomerID           *string    `json:"customer_id"`
	CustomerName         string     `json:"customer_name"`
	VehicleCategoryID    *string    `json:"vehicle_category_id"`
	VehicleCategoryName  string     `json:"vehicle_category_name"`
	VehicleUsageID       *string    `json:"vehicle_usage_id"`
	VehicleUsageName     string     `json:"vehicle_usage_name"`
	Plate                string     `json:"plate"`
	EmployeeID           *string    `json:"employee_id"`
	EmployeeName         string     `json:"employee_name"`
	Status               string     `json:"status"`
	SubtotalXOF          int        `json:"subtotal_xof"`
	DiscountXOF          int        `json:"discount_xof"`
	DiscountReason       string     `json:"discount_reason"`
	TotalXOF             int        `json:"total_xof"`
	Notes                string     `json:"notes"`
	IncidentNote         *string    `json:"incident_note"`
	IncidentReportedAt   *time.Time `json:"incident_reported_at"`
	IncidentReporterName *string    `json:"incident_reporter_name"`
	CancelReason         *string    `json:"cancel_reason"`
	CreatedBy            string     `json:"created_by"`
	CreatedByName        string     `json:"created_by_name"`
	CreatedAt            time.Time  `json:"created_at"`
	StartedAt            *time.Time `json:"started_at"`
	CompletedAt          *time.Time `json:"completed_at"`
	CanceledAt           *time.Time `json:"canceled_at"`
	BusinessDay          string     `json:"business_day"`
	// Le passage par le contrôle qualité et la remise du véhicule laissent une
	// trace datée : « le contrôle a été fait » et « le véhicule est prêt » ne se
	// déduisent pas du statut courant, ils se constatent.
	QualityCheckedAt *time.Time      `json:"quality_checked_at"`
	ReadyAt          *time.Time      `json:"ready_at"`
	DeliveredAt      *time.Time      `json:"delivered_at"`
	Items            []WashOrderItem `json:"items"`
}

type CashSession struct {
	ID              string     `json:"id"`
	OrganizationID  string     `json:"organization_id"`
	StationID       string     `json:"station_id"`
	StationName     string     `json:"station_name"`
	Status          string     `json:"status"`
	OpenedBy        string     `json:"opened_by"`
	OpenedByName    string     `json:"opened_by_name"`
	OpenedAt        time.Time  `json:"opened_at"`
	OpeningFloatXOF int        `json:"opening_float_xof"`
	ExpectedCashXOF int        `json:"expected_cash_xof"`
	CountedCashXOF  *int       `json:"counted_cash_xof"`
	DifferenceXOF   *int       `json:"difference_xof"`
	CashInXOF       int        `json:"cash_in_xof"`
	MobileMoneyXOF  int        `json:"mobile_money_xof"`
	OtherInXOF      int        `json:"other_in_xof"`
	ExpensesXOF     int        `json:"expenses_xof"`
	Operations      int        `json:"operations"`
	PaymentCount    int        `json:"payment_count"`
	ExpenseCount    int        `json:"expense_count"`
	Notes           string     `json:"notes"`
	CloseNotes      string     `json:"close_notes"`
	ReopenCount     int        `json:"reopen_count"`
	BusinessDay     string     `json:"business_day"`
	ClosedBy        *string    `json:"closed_by"`
	ClosedByName    *string    `json:"closed_by_name"`
	ClosedAt        *time.Time `json:"closed_at"`
}

type Payment struct {
	ID             string  `json:"id"`
	OrganizationID string  `json:"organization_id"`
	StationID      string  `json:"station_id"`
	WashOrderID    *string `json:"wash_order_id"`
	CustomerID     *string `json:"customer_id"`
	Kind           string  `json:"kind"`
	Method         string  `json:"method"`
	// PaymentMethodID : le moyen **déclaré** par la station (espèces, Orange
	// Money, Wave…). Vide pour les encaissements antérieurs à la table — on ne
	// leur invente pas une origine après coup.
	PaymentMethodID   *string   `json:"payment_method_id"`
	PaymentMethodName string    `json:"payment_method_name"`
	AmountXOF         int       `json:"amount_xof"`
	Reference         string    `json:"reference"`
	Note              string    `json:"note"`
	PaidAt            time.Time `json:"paid_at"`
	BusinessDay       string    `json:"business_day"`
	CashSessionID     *string   `json:"cash_session_id"`
	ReceivedBy        string    `json:"received_by"`
	ReceivedByName    string    `json:"received_by_name"`
	Status            string    `json:"status"`
	VoidReason        *string   `json:"void_reason"`
}

type Expense struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organization_id"`
	StationID      string    `json:"station_id"`
	Category       string    `json:"category"`
	Label          string    `json:"label"`
	AmountXOF      int       `json:"amount_xof"`
	Method         string    `json:"method"`
	Supplier       string    `json:"supplier"`
	Reference      string    `json:"reference"`
	Note           string    `json:"note"`
	SpentAt        time.Time `json:"spent_at"`
	BusinessDay    string    `json:"business_day"`
	CashSessionID  *string   `json:"cash_session_id"`
	RecordedBy     string    `json:"recorded_by"`
	RecordedByName string    `json:"recorded_by_name"`
	Status         string    `json:"status"`
	VoidReason     *string   `json:"void_reason"`
	CreatedAt      time.Time `json:"created_at"`
}

type AuditEntry struct {
	ID             int64          `json:"id"`
	OrganizationID *string        `json:"organization_id"`
	StationID      *string        `json:"station_id"`
	ActorName      string         `json:"actor_name"`
	Action         string         `json:"action"`
	Entity         string         `json:"entity"`
	EntityID       string         `json:"entity_id"`
	Details        map[string]any `json:"details"`
	CreatedAt      time.Time      `json:"created_at"`
}
