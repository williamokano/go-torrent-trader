package model

import "time"

// Bonus store item kinds. invite and upload_credit are implemented;
// freeleech_ticket and double_upload are catalogue placeholders whose purchase
// is rejected until their effects exist.
const (
	BonusKindInvite          = "invite"
	BonusKindUploadCredit    = "upload_credit"
	BonusKindFreeleechTicket = "freeleech_ticket"
	BonusKindDoubleUpload    = "double_upload"
)

// Bonus ledger reasons.
const (
	BonusReasonSeeding     = "seeding"
	BonusReasonPurchase    = "purchase"
	BonusReasonAdminAdjust = "admin_adjust"
	// BonusReasonHnRClear marks a spend that paid off a hit-and-run
	// obligation. RefID on the transaction is the hnr_records id cleared.
	BonusReasonHnRClear = "hnr_clear"
)

// BonusStoreItem is one purchasable entry in the bonus store catalogue.
// RewardAmount's unit depends on Kind: invites for invite, bytes for
// upload_credit, tickets for freeleech_ticket, hours for double_upload.
type BonusStoreItem struct {
	ID           int64
	Slug         string
	Kind         string
	Name         string
	Description  string
	Price        int64
	RewardAmount int64
	Enabled      bool
	Sort         int
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// BonusTransaction is one append-only ledger entry for a user's points.
// RefID is the store item for purchases and the acting admin for adjustments.
type BonusTransaction struct {
	ID        int64
	UserID    int64
	Delta     int64
	Reason    string
	RefID     *int64
	CreatedAt time.Time
}
