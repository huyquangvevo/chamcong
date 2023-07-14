package main

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

const (
	ALERT_NOT_CHECK_IN   = "Không chấm công"
	ALERT_NOT_CHECK_OUT  = "Không chấm giờ ra"
	ALERT_CHECK_IN_LATE  = "Đi muộn"
	ALERT_CHECK_OUT_SOON = "Về sớm"
	ALERT_NOT_ENOUGH     = "Không đủ giờ làm"
)

type Attendance struct {
	ID           primitive.ObjectID `bson:"_id"`
	DepartmentId uint64             `bson:"department_id"`
	UserId       uint64             `bson:"user_id"`
	TimeCheckIn  time.Time          `bson:"time_checkin"`
	TimeCheckOut time.Time          `bson:"time_checkout"`
	Message      string
}

type User struct {
	Username     string
	Email        string
	Id           uint64
	DepartmentId uint64
	PositionId   uint64
	ManagerId    uint64
}

type Department struct {
	DepartmentId  uint64
	Total         uint64
	Work          uint64
	CheckInLate   uint64
	CheckOutSoon  uint64
	NotCheckIn    uint64
	NotCheckOut   uint64
	NotEnoughWork uint64
}
