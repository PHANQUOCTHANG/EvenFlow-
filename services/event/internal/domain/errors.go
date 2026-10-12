package domain

import "errors"

var (
	ErrEventNotFound           = errors.New("event not found")
	ErrTierNotFound            = errors.New("ticket tier not found")
	ErrInvalidStateTransition  = errors.New("invalid event state transition")
	ErrNoValidTicketTiers      = errors.New("event must have at least one ticket tier with quota > 0 and price >= 0")
	ErrSaleStartTimeTooEarly   = errors.New("sale_start_at must be at least 30 minutes after approval time")
	ErrCannotReduceQuotaOnSale = errors.New("cannot reduce ticket tier quota after event is ON_SALE")
	ErrDuplicateTierName       = errors.New("ticket tier name already exists for this event")
	ErrEventNotEditable        = errors.New("event can only be edited in DRAFT or SCHEDULED state")
	ErrInvalidQuota            = errors.New("quota must be greater than 0")
	ErrInvalidPrice            = errors.New("price must be greater than or equal to 0")
	ErrInvalidEventDates       = errors.New("event starts_at must be after sale_start_at")
	ErrInvalidTitle            = errors.New("event title cannot be empty")
	ErrUnauthorizedOrganizer   = errors.New("unauthorized organizer for this event")
)
