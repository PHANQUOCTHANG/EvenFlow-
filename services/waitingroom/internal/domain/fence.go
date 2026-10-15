package domain

import "errors"

// ErrFenced: leader cu bi tu choi vi epoch cua no da cu (mot leader moi da len). Controller gap
// loi nay phai dung ngay, KHONG thu lai: moi lenh ghi tiep theo cung se bi chan, va tiep tuc la
// dung vao quyen cua leader moi.
var ErrFenced = errors.New("controller da bi fencing: co leader moi")
