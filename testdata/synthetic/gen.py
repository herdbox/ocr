# Renders phone-like settings screens with known text lines (ground truth) into
# the folder given as argument. Run on Debian/Ubuntu with fonts-roboto,
# fonts-noto-core and fonts-dejavu-core installed, and Pillow.
import json, random, sys
from PIL import Image, ImageDraw, ImageFont

random.seed(7)
OUT = sys.argv[1] if len(sys.argv) > 1 else "."
LINES = """Cài đặt
Đăng nhập tài khoản
Mạng di động
Kết nối và chia sẻ
Hình nền và kiểu
Âm thanh và rung
Thông báo và thanh trạng thái
Màn hình khóa
Mật khẩu và bảo mật
Quyền riêng tư
Vị trí
Pin và hiệu năng
Ứng dụng
Bộ nhớ trong
Tài khoản và đồng bộ
Hệ thống
Giới thiệu về điện thoại
Cập nhật phần mềm
Trợ năng
Ngôn ngữ và khu vực
Ngày và giờ
Sao lưu và khôi phục
Đặt lại điện thoại
Chế độ máy bay
Điểm truy cập cá nhân
Dữ liệu di động đang tắt
Đã kết nối
Không có thẻ SIM
Tự động xoay màn hình
Độ sáng
Chế độ tối
Cỡ chữ và hiển thị
Bàn phím ảo
Gỡ cài đặt
Buộc dừng
Xóa bộ nhớ đệm
Quản lý ứng dụng
Cấp quyền truy cập
Tiếp tục
Hủy bỏ
Đồng ý
Bỏ qua
Thử lại sau
Nhập mã xác minh
Gửi lại mã sau 59 giây
Số điện thoại của bạn
Quên mật khẩu?
Tạo tài khoản mới
Đăng xuất
Lưu thay đổi
Tìm kiếm cài đặt
Phiên bản 12.4.1
Dung lượng còn trống 23,5 GB
Bluetooth
Wi-Fi
Google
Facebook
TikTok Shop
Zalo
Chia sẻ vị trí trực tiếp
Thời gian sử dụng thiết bị
Chuyển tiền nhanh 24/7
Ưu đãi hấp dẫn hôm nay
Xem tất cả bình luận
Thích
Theo dõi
Nhắn tin
Đã gửi lúc 08:45
Người lạ muốn kết bạn
Báo cáo vi phạm
Tắt tiếng thông báo
Giỏ hàng của tôi
Thanh toán khi nhận hàng
Miễn phí vận chuyển
Đánh giá sản phẩm
Mã giảm giá đã được áp dụng""".splitlines()

FONTS = ["/usr/share/fonts/truetype/roboto/unhinted/RobotoTTF/Roboto-Regular.ttf",
         "/usr/share/fonts/truetype/roboto/unhinted/RobotoTTF/Roboto-Medium.ttf",
         "/usr/share/fonts/truetype/noto/NotoSans-Regular.ttf",
         "/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf"]

def screen(idx, dark):
    W, H = 1080, 2400
    bg, fg, sub = ((18, 18, 18), (235, 235, 235), (160, 160, 160)) if dark else ((248, 248, 248), (25, 25, 25), (110, 110, 110))
    im = Image.new("RGB", (W, H), bg)
    d = ImageDraw.Draw(im)
    font = FONTS[idx % len(FONTS)]
    y, gt = 180, []
    picks = random.sample(LINES, 14)
    for i, t in enumerate(picks):
        size = random.choice([40, 44, 48, 52]) if i else 72
        col = fg if random.random() > .25 else sub
        f = ImageFont.truetype(font, size)
        x = 60 if i else 60
        bb = d.textbbox((x, y), t, font=f)
        d.text((x, y), t, font=f, fill=col)
        gt.append({"text": t, "box": list(bb)})
        y = bb[3] + random.choice([70, 90, 110])
        if y > H - 150: break
    name = f"syn{idx:02d}.png"
    im.save(f"{OUT}/{name}")
    return name, gt

out = {}
for i in range(12):
    n, g = screen(i, i % 3 == 2)
    out[n] = g
json.dump(out, open(f"{OUT}/gt.json", "w"), ensure_ascii=False, indent=1)
print(len(out), sum(len(v) for v in out.values()))
