package encoder

import "github.com/speps/go-hashids/v2"

func Encode(id uint64, salt string) (string, error) {
	hd := hashids.NewData()
	hd.Salt = salt
	hd.MinLength = 6

	h, err := hashids.NewWithData(hd)
	if err != nil {
		return "", err
	}
	return h.Encode([]int{int(id)})
}
