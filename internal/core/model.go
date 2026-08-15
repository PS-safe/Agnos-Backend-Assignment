package core

import "time"

type Hospital struct {
	ID   string
	Code string
	Name string
}

type Staff struct {
	ID           string
	HospitalID   string
	HospitalCode string
	Username     string
	PasswordHash string
	CreatedAt    time.Time
}

type AuthContext struct {
	StaffID      string
	HospitalID   string
	HospitalCode string
	Username     string
}

type Patient struct {
	ID            string
	HospitalID    string
	FirstNameTH   string
	MiddleNameTH  string
	LastNameTH    string
	FirstNameEN   string
	MiddleNameEN  string
	LastNameEN    string
	DateOfBirth   time.Time
	PatientHN     string
	NationalID    string
	PassportID    string
	PhoneNumber   string
	Email         string
	Gender        string
	SourceSystem  string
	SourceUpdated time.Time
}
