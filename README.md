# order-demo-backend

Backend ของระบบสั่งของพร้อมจ่ายด้วย PromptPay (ตัวอย่าง: ร้านกาแฟ) เขียนด้วย Go, Gin และ PostgreSQL
ใช้คู่กับแอป Flutter [order-demo-app](https://github.com/kitchu-studio/order-demo-app)

## ทำอะไรได้

- สมัคร/เข้าสู่ระบบ ด้วย JWT (access 15 นาที + refresh 30 วัน), รหัสผ่านเก็บแบบ bcrypt
- ดูเมนู (public) และสั่งของ ราคาคิดฝั่งเซิร์ฟเวอร์เสมอ ไม่เชื่อราคาจากแอป
- สร้าง payload PromptPay QR (EMVCo + CRC16) จากยอดออเดอร์ ให้แอปวาดเป็น QR
- ติดตามสถานะ `pending_payment → paid → preparing → ready → completed` (ยกเลิกได้ก่อนร้านรับเงิน)
- ฝั่งร้าน (admin): ดูคิวออเดอร์ เปลี่ยนสถานะทีละขั้น และกดของหมด/มีของ

> ไม่ได้ต่อกับธนาคาร: ร้านเป็นคนกดยืนยันว่าได้รับเงินแล้ว เหมาะกับเดโมและร้านเล็ก
> ถ้าจะรับเงินอัตโนมัติ ต้องต่อ payment gateway เพิ่ม

## รัน

```bash
cp .env.example .env        # แก้ค่าทุกตัว โดยเฉพาะ JWT_SECRET, PROMPTPAY_ID, ADMIN_EMAILS
docker compose -f docker-compose.yml -f docker-compose.dev.yml up -d --build
curl http://localhost:8080/healthz
```

ไฟล์ `docker-compose.dev.yml` เปิดพอร์ต 8080 แบบ HTTP สำหรับต่อกับ Android emulator เท่านั้น ห้ามใช้บนเซิร์ฟเวอร์จริง

## API

| Method | Path | ใคร |
|---|---|---|
| POST | `/api/v1/auth/register`, `/login`, `/refresh` | ทุกคน |
| GET | `/api/v1/shop`, `/api/v1/products` | ทุกคน |
| GET | `/api/v1/me` | ล็อกอิน |
| POST | `/api/v1/orders` `{items:[{product_id,qty}], note}` | ล็อกอิน |
| GET | `/api/v1/orders`, `/api/v1/orders/:id` | ล็อกอิน (ของตัวเอง) |
| POST | `/api/v1/orders/:id/cancel` | ล็อกอิน (ก่อนจ่าย) |
| GET | `/api/v1/admin/orders?status=` | admin |
| POST | `/api/v1/admin/orders/:id/status` `{status}` | admin |
| PATCH | `/api/v1/admin/products/:id` `{available}` | admin |

ออเดอร์ที่ยังรอจ่ายจะมี `payment.payload` เป็นข้อความสำหรับสร้าง QR

## ทดสอบ

```bash
go test ./...
```

## ขึ้นเซิร์ฟเวอร์

โครงเดียวกับ booking-demo-backend: Docker Compose (Caddy + Go + Postgres) บน EC2 ของลูกค้า
และ GitHub Actions deploy ผ่าน SSM (ดู `.github/workflows/deploy.yml`)
