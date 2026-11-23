/** Structured translations. Screens not yet migrated remain in English. */
const en = {
  welcomeTitle: "Say cheese!", welcomeSub: "Pick a frame, strike a pose, and get your photo strip.", welcomeBlocked: "We will be right with you",
  wait: "One moment…", takePhotos: "Take photos", gettingReady: "Getting ready for you", chooseFrame: "Choose a frame to begin",
  tidy: "We are still tidying up the last session.", tryAgain: "Try again",
  dashboard: "Dashboard", prepare: "Prepare Booth", camera: "Camera", printer: "Printer", frames: "Frames", gallery: "Gallery", photo: "Photo", video: "Video", settings: "Settings", diagnostics: "Diagnostics",
  frameStep: "Frame", shootStep: "Shoot", reviewStep: "Review", doneStep: "Done",
} as const;
export type TranslationKey = keyof typeof en;
const vi: Record<TranslationKey, string> = {
  welcomeTitle: "Cười lên nào!", welcomeSub: "Chọn khung, tạo dáng và nhận dải ảnh của bạn.", welcomeBlocked: "Vui lòng đợi trong giây lát",
  wait: "Xin chờ một chút…", takePhotos: "Chụp ảnh", gettingReady: "Đang chuẩn bị cho bạn", chooseFrame: "Chọn một khung hình để bắt đầu",
  tidy: "Chúng tôi đang hoàn tất phiên trước.", tryAgain: "Thử lại",
  dashboard: "Tổng quan", prepare: "Chuẩn bị buồng chụp", camera: "Máy ảnh", printer: "Máy in", frames: "Khung hình", gallery: "Thư viện", photo: "Ảnh", video: "Video", settings: "Cài đặt", diagnostics: "Chẩn đoán",
  frameStep: "Khung", shootStep: "Chụp", reviewStep: "Xem lại", doneStep: "Hoàn tất",
};
export function translate(language: string | undefined, key: TranslationKey): string {
  return (language === "vi" ? vi[key] : undefined) ?? en[key];
}
