// SPDX-License-Identifier: Apache-2.0
package xiaobot

import (
	"context"
	"errors"
	"xiaobot/miservice"
)

// BindVerifiedSpeaker reuses the account and exact device from the authorized list.
func (m *XiaoMi) BindVerifiedSpeaker(ctx context.Context, account *miservice.Account, device miservice.DeviceData) error {
	if device.MiotDID == "" || device.MiotDID != m.config.MiDID || device.DeviceID == "" {
		return errors.New("verified speaker does not match the selected DID")
	}
	if m.config.UseCommand {
		if err := account.RenewCachedSID(ctx, miservice.MIIO_SID); err != nil {
			return err
		}
	}
	m.account = account
	m.minaService = miservice.NewAIService(account)
	m.miioService = miservice.NewIOService(account, nil)
	m.deviceID = device.DeviceID
	m.MacAddr = normalizeMAC(device.Mac)
	return nil
}
func (m *XiaoMi) RefreshMIoTCachedSID(ctx context.Context) error {
	if m.account == nil {
		return errors.New("Xiaomi account unavailable")
	}
	return m.account.RenewCachedSID(ctx, miservice.MIIO_SID)
}
