package domain

// Lane la lan xep hang cua mot lan ghi danh (anti-bot, contracts 2). Gia tri trung voi ma lane cua join.lua.
type Lane string

const (
	LaneNormal Lane = "n" // lan binh thuong (develop)
	LaneSlow   Lane = "s" // slow lane: cung response, xep sau P vi tri (D5)
)

// StateBlocked la ket qua noi bo cua JoinLane khi co chan 24 h dang ton tai (D6). Handler doi no thanh 403,
// khong bao gio tra cho client.
const StateBlocked State = "BLOCKED"
