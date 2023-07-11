package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"database/sql"

	"github.com/joho/godotenv"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"gopkg.in/gomail.v2"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

type Timekeeping struct {
	SqlDB          *gorm.DB
	MongoDB        *mongo.Database
	Mail           *gomail.Dialer
	TimeIn         string
	TimeOut        string
	MailContentTmp string
	TimeWork       int
}

func NewTimekeeping() *Timekeeping {
	timeWork, err := strconv.Atoi(os.Getenv("TIME_WORK"))
	if err != nil {
		log.Fatal("Error when read TIME_WORK: ", err)
	}

	d := Timekeeping{
		TimeIn:         os.Getenv("TIME_IN"),
		TimeOut:        os.Getenv("TIME_OUT"),
		TimeWork:       timeWork,
		MailContentTmp: os.Getenv("MAIL_CONTENT_TPL"),
	}

	// init db
	DB_HOST := os.Getenv("DB_HOST")
	DB_PORT := os.Getenv("DB_PORT")
	DB_USER := os.Getenv("DB_USER")
	DB_PASS := os.Getenv("DB_PASS")
	DB_NAME := os.Getenv("DB_NAME")

	sqlDsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s", DB_USER, DB_PASS, DB_HOST, DB_PORT, DB_NAME)
	sqlDB, err := sql.Open("mysql", sqlDsn)
	if err != nil {
		log.Fatal("Error when connect to db: ", err)
	}

	gormDB, err := gorm.Open(mysql.New(mysql.Config{
		Conn: sqlDB,
	}), &gorm.Config{})

	if err != nil {
		log.Fatal("Error when open gorm: ", err)
	}
	d.SqlDB = gormDB

	// init mail
	MAIL_HOST := os.Getenv("MAIL_HOST")
	MAIL_USER := os.Getenv("MAIL_USER")
	MAIL_PASS := os.Getenv("MAIL_PASS")
	MAIL_PORT, err := strconv.Atoi(os.Getenv("MAIL_PORT"))
	if err != nil {
		log.Fatal("Error when parse port mail from env: ", err)
	}

	d.Mail = gomail.NewDialer(MAIL_HOST, MAIL_PORT, MAIL_USER, MAIL_PASS)
	d.Mail.TLSConfig = &tls.Config{InsecureSkipVerify: true}

	uri := os.Getenv("MONGODB_URI")
	if uri == "" {
		log.Fatal("You must set your 'MONGODB_URI' environmental variable")
	}
	client, err := mongo.Connect(context.TODO(), options.Client().ApplyURI(uri))
	if err != nil {
		panic(err)
	}
	d.MongoDB = client.Database(os.Getenv("MONGODB_NAME"))
	return &d
}

func (d *Timekeeping) sendMail(to []string, cc []string, mailContent string) {
	m := gomail.NewMessage()
	m.SetHeader("From", os.Getenv("MAIL_USER"))
	m.SetHeader("To", to...)
	if len(cc) > 0 {
		m.SetHeader("Cc", cc...)
	}
	m.SetHeader("Subject", os.Getenv("MAIL_SUBJECT"))
	m.SetBody("text/html", mailContent)

	// Send the email to Bob, Cora and Dan.
	if err := d.Mail.DialAndSend(m); err != nil {
		log.Default().Printf("Error when send mail: ", err)
	}
}

func (d *Timekeeping) alert() {

	today := time.Now()
	timeInStr := today.Format("2006-01-02") + " " + d.TimeIn
	timeIn, err := time.Parse("2006-01-02 15:04", timeInStr)
	if err != nil {
		panic(err)
	}

	timeOutStr := today.Format("2006-01-02") + " " + d.TimeOut
	timeOut, err := time.Parse("2006-01-02 15:04", timeOutStr)
	if err != nil {
		panic(err)
	}

	var departments []Department
	d.SqlDB.Model(&User{}).Select("department_id, count(id) as total").Group("department_id").Find(&departments)

	for _, depm := range departments {

		depm.CheckInLate = 0
		depm.CheckOutSoon = 0
		depm.NotCheckIn = 0

		lstEmpEmail := []string{}
		var managerEmail string

		var users []User
		d.SqlDB.Where("department_id = ?", depm.DepartmentId).Find(&users)

		filter := bson.M{
			"department_id": depm.DepartmentId,
		}

		atdCol := d.MongoDB.Collection("attendance")

		checkInUser := map[uint64]Attendance{}

		var attendances []Attendance
		cur, err := atdCol.Find(context.TODO(), filter)
		if err = cur.All(context.TODO(), &attendances); err != nil {
			panic(err)
		}

		// lstUserId := make([]uint64, len(attendances))
		for _, atd := range attendances {
			// lstUserId = append(lstUserId, atd.UserId)

			if atd.TimeCheckIn.IsZero() {
				depm.NotCheckIn += 1
				atd.Message = ALERT_NOT_CHECK_IN
			} else if atd.TimeCheckOut.IsZero() {
				depm.NotCheckOut += 1
				atd.Message = ALERT_NOT_CHECK_OUT
			} else if atd.TimeCheckIn.After(timeIn) {
				depm.CheckInLate += 1
				atd.Message = ALERT_CHECK_IN_LATE
			} else if atd.TimeCheckOut.Before(timeOut) {
				depm.CheckOutSoon += 1
				atd.Message = ALERT_CHECK_OUT_SOON
			} else if atd.TimeCheckOut.Hour()-atd.TimeCheckIn.Hour() < d.TimeWork {
				depm.NotEnoughWork += 1
				atd.Message = ALERT_NOT_ENOUGH
			}
			checkInUser[atd.UserId] = atd
		}

		depm.Work = depm.Total - (depm.NotCheckIn + depm.NotCheckOut + depm.CheckInLate + depm.CheckOutSoon)

		for _, user := range users {
			if user.PositionId == 1 {
				managerEmail = user.Email
			} else {
				lstEmpEmail = append(lstEmpEmail, user.Email)
			}
		}

		mailContent := d.MailContentTmp
		mailContent = strings.ReplaceAll(mailContent, "$DEPT_NAME", managerEmail)
		mailContent = strings.ReplaceAll(mailContent, "$NOT_CHECK_IN", string(depm.NotCheckIn))
		mailContent = strings.ReplaceAll(mailContent, "$CHECK_IN_LATE", string(depm.CheckInLate))
		mailContent = strings.ReplaceAll(mailContent, "$CHECK_OUT_SOON", string(depm.CheckOutSoon))
		mailContent = strings.ReplaceAll(mailContent, "$NOT_ENOUGH_WORK", string(depm.NotEnoughWork))
		d.sendMail([]string{managerEmail}, lstEmpEmail, mailContent)

	}
}

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found")
	}

	timeKeeping := NewTimekeeping()
	timeKeeping.alert()
}
