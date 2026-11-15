package main

import "time"

const timeLayout = "2006-01-02 15:04:05"

func nowText() string {
	return time.Now().Format(timeLayout)
}

type User struct {
	ID           int64
	Account      string
	DisplayName  string
	Phone        string
	WechatOpenID string
	Role         string
	Status       string
	CreatedAt    string
	LastLoginAt  *string
}

type Software struct {
	ID         int64
	Name       string
	WingetID   string
	Purpose    string
	Summary    string
	AISoftware string
	Required   bool
	Notes      string
}

type Version struct {
	ID           int64
	SoftwareID   int64
	Version      string
	FileName     string
	FilePath     string
	FileSize     int64
	SHA256       string
	ScanStatus   string
	ScanDetail   string
	ScannedAt    *string
	PublishedAt  string
	IsCurrent    bool
	ReleaseNotes string
	Status       string
}

type Note struct {
	Version     string `json:"version"`
	Title       string `json:"title"`
	Detail      string `json:"detail"`
	PublishedAt string `json:"publishedAt"`
}

type UserDTO struct {
	ID          int64   `json:"id"`
	Account     string  `json:"account"`
	DisplayName string  `json:"displayName"`
	Phone       string  `json:"phone"`
	PhoneText   string  `json:"phoneText"`
	Role        string  `json:"role"`
	Status      string  `json:"status"`
	CreatedAt   string  `json:"createdAt"`
	LastLoginAt *string `json:"lastLoginAt"`
}

type VersionDTO struct {
	Version      string  `json:"version"`
	FileName     string  `json:"fileName"`
	FileSize     int64   `json:"fileSize"`
	SHA256       string  `json:"sha256"`
	ScanStatus   string  `json:"scanStatus"`
	ScanDetail   string  `json:"scanDetail"`
	ScannedAt    *string `json:"scannedAt"`
	PublishedAt  string  `json:"publishedAt"`
	ReleaseNotes string  `json:"releaseNotes"`
	IsCurrent    bool    `json:"isCurrent"`
	Status       string  `json:"status"`
}

type SoftwareDTO struct {
	ID         int64       `json:"id"`
	Name       string      `json:"name"`
	Purpose    string      `json:"purpose"`
	Summary    string      `json:"summary"`
	AISoftware string      `json:"aiSoftware"`
	Required   bool        `json:"required"`
	WingetID   string      `json:"wingetId"`
	Notes      string      `json:"notes"`
	Current    *VersionDTO `json:"current"`
	Previous   *VersionDTO `json:"previous"`
}

func toUserDTO(user User) UserDTO {
	phone := user.Phone
	return UserDTO{
		ID:          user.ID,
		Account:     user.Account,
		DisplayName: user.DisplayName,
		Phone:       phone,
		PhoneText:   maskPhone(phone),
		Role:        user.Role,
		Status:      user.Status,
		CreatedAt:   user.CreatedAt,
		LastLoginAt: user.LastLoginAt,
	}
}

func toVersionDTO(version Version) VersionDTO {
	return VersionDTO{
		Version:      version.Version,
		FileName:     version.FileName,
		FileSize:     version.FileSize,
		SHA256:       version.SHA256,
		ScanStatus:   version.ScanStatus,
		ScanDetail:   version.ScanDetail,
		ScannedAt:    version.ScannedAt,
		PublishedAt:  version.PublishedAt,
		ReleaseNotes: version.ReleaseNotes,
		IsCurrent:    version.IsCurrent,
		Status:       version.Status,
	}
}

func maskPhone(phone string) string {
	if len(phone) == 11 {
		return phone[:3] + "****" + phone[7:]
	}
	return phone
}
