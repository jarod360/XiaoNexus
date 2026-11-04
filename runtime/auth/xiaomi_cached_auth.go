// SPDX-License-Identifier: Apache-2.0
package xiaobot
import("context";"errors";"xiaobot/miservice")
func(m *XiaoMi) RefreshMIoTCachedSID(ctx context.Context)error{
 if m.account==nil{return errors.New("Xiaomi account unavailable")}
 return m.account.RenewCachedSID(ctx,miservice.MIIO_SID)
}
