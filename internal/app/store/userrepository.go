package store

import "github.com/Gorynychdo/aster_go/internal/app/model"

type UserRepository struct {
	store *Store
}

func (u *UserRepository) Find(endpointID string) (*model.User, error) {
	user := &model.User{}

	if err := u.store.db.QueryRow(
		"SELECT id, tel, device_token, endpoint_id FROM users WHERE endpoint_id = ?", endpointID,
	).Scan(&user.ID, &user.PhoneNumber, &user.DeviceToken, &user.EndpointID); err != nil {
		return nil, err
	}

	return user, nil
}
