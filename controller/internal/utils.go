package internal

import (
	"crypto/rand"
	log "github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"io"
)

func ContainsObjectID(s []primitive.ObjectID, str primitive.ObjectID) bool {
	for _, v := range s {
		if v == str {
			return true
		}
	}
	return false
}

func GeneratePIN(max int) string {
	var table = [...]byte{'1', '2', '3', '4', '5', '6', '7', '8', '9', '0'}
	b := make([]byte, max)
	n, err := io.ReadAtLeast(rand.Reader, b, max)
	if n != max {
		log.Error(err)
		return "6969420" // the gamer numbers (XD rawr)
	}
	for i := 0; i < len(b); i++ {
		b[i] = table[int(b[i])%len(table)]
	}
	return string(b)
}
