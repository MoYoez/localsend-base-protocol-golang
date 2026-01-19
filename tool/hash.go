package tool

import "github.com/google/uuid"

func GenerateRandomUUID() string {
	return uuid.New().String()
}
