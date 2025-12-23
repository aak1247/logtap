package project

import (
	"errors"
	"fmt"
)

func ParseID(projectID string) (int, error) {
	var id int
	_, err := fmt.Sscanf(projectID, "%d", &id)
	if err != nil || id <= 0 {
		return 0, errors.New("invalid projectId")
	}
	return id, nil
}

