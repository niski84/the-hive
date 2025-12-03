// Copyright (c) 2025 Northbound System
// Author: Nicholas Skitch
package main

import (
	"github.com/the-hive/internal/server"
)

// notificationAdapter adapts WebSocketManager to implement worker.NotificationSender
type notificationAdapter struct {
	wm *server.WebSocketManager
}

// SendNotification implements worker.NotificationSender interface
func (a *notificationAdapter) SendNotification(clientID, notificationType, message, level string) error {
	return a.wm.SendNotificationRaw(clientID, notificationType, message, level)
}

