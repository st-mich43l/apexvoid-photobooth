import type { Dict } from "./en";

/** Vietnamese copy. Must match the English dictionary key for key (enforced by the `Dict` type). */
export const vi: Dict = {
  meta: {
    siteName: "ApexVoid Photobooth",
    tagline: "Phần mềm chụp ảnh photobooth lưu trữ cục bộ cho Windows",
  },
  a11y: {
    skip: "Chuyển đến nội dung",
    mainNav: "Điều hướng chính",
    footerNav: "Chân trang",
    openMenu: "Mở menu",
    closeMenu: "Đóng menu",
    theme: "Giao diện màu",
    themeLight: "Sáng",
    themeDark: "Tối",
    themeSystem: "Theo hệ thống",
    language: "Ngôn ngữ",
    switchTo: "Đọc trang này bằng tiếng Việt",
    newTab: "(mở trong thẻ mới)",
    home: "Trang chủ ApexVoid Photobooth",
  },
  nav: {
    features: "Tính năng",
    download: "Tải xuống",
    docs: "Tài liệu",
    community: "Cộng đồng",
    releases: "Phiên bản",
    github: "GitHub",
  },
  common: {
    downloadWindows: "Tải cho Windows",
    exploreFeatures: "Khám phá tính năng",
    viewOnGithub: "Xem trên GitHub",
    openGuide: "Mở hướng dẫn",
    screenshotNote:
      "Ảnh chụp màn hình thật của ứng dụng, được tạo trong môi trường thử nghiệm trên trình duyệt với máy ảnh mô phỏng. Chúng không chứng minh rằng máy ảnh hay máy in nào hoạt động.",
    platform: {
      os: "Windows 10 và 11",
      arch: "x64",
      portable: "Bản di động — một tệp chạy duy nhất",
      webview: "Cần Microsoft Edge WebView2 Runtime",
    },
    status: {
      integration: "Đã tích hợp phần mềm",
      verified: "Đã kiểm chứng trên thiết bị",
      testing: "Cần thử nghiệm phần cứng",
    },
  },
  home: {
    title: "ApexVoid Photobooth — phần mềm photobooth lưu trữ cục bộ cho Windows",
    description:
      "Ứng dụng photobooth hiện đại, lưu trữ cục bộ cho Windows: phiên chụp cảm ứng có hướng dẫn, khung ảnh tùy chỉnh, in qua máy in Windows và thư viện ảnh nằm ngay trên máy tính của bạn.",
    hero: {
      eyebrow: "Phần mềm photobooth cho Windows",
      title: "Lưu giữ khoảnh khắc.",
      titleAccent: "Tạo nên kỷ niệm.",
      lede: "Ứng dụng photobooth hiện đại, lưu trữ cục bộ, dành cho nhà sáng tạo, studio, sự kiện và những trải nghiệm khó quên.",
      imageAlt: "Chọn khung ở Chế độ Booth: lưới các mẫu dải ảnh, một mẫu được chọn và bản xem trước lớn bên cạnh.",
    },
    experience: {
      eyebrow: "Hai trải nghiệm, một ứng dụng",
      title: "Sân khấu nhẹ nhàng cho khách. Bàn làm việc rõ ràng cho người vận hành.",
      booth: {
        label: "Chế độ Booth",
        title: "Khách là người dẫn dắt.",
        body: "Hành trình toàn màn hình, ưu tiên cảm ứng: khách chọn khung trước, nhìn thấy mình trực tiếp, đếm ngược, xem lại từng ảnh, chụp lại những ảnh chưa ưng và nhận dải ảnh đã ghép hoàn chỉnh.",
        points: [
          "Chọn khung trước khi máy ảnh khởi động",
          "Xem trực tiếp, đếm ngược lớn và phản hồi khi chụp",
          "Xem lại và chụp lại từng ảnh",
          "Ghép ảnh vào khung đã chọn, sẵn sàng để in",
        ],
        imageAlt: "Đếm ngược ở Chế độ Booth: số 3 lớn trên hình xem trực tiếp, khung đã chọn ở bên cạnh.",
        secondAlt: "Kết quả ở Chế độ Booth: dải ảnh hoàn chỉnh với các nút In, Đổi thiết kế và Hoàn tất.",
      },
      studio: {
        label: "Chế độ Studio",
        title: "Mọi thứ người vận hành cần, trước khi khách đến.",
        body: "Thiết lập máy ảnh và máy in, quản lý khung, duyệt và xuất các phiên chụp, kiểm tra trạng thái sẵn sàng — màn hình của khách được tách riêng và có thể khóa bằng mã PIN thoát.",
        points: [
          "Kiểm tra máy ảnh, máy in và trạng thái sẵn sàng",
          "Bộ sưu tập khung và trình tạo khung",
          "Thư viện ảnh cục bộ có tìm kiếm và xuất",
          "Chẩn đoán và gói hỗ trợ ngoại tuyến",
        ],
        imageAlt: "Bảng điều khiển Studio với trạng thái máy ảnh, máy in, dung lượng lưu trữ và các phiên gần đây.",
      },
    },
    groups: {
      eyebrow: "Ứng dụng làm được gì",
      title: "Xoay quanh khoảnh khắc, không phải menu.",
      items: [
        {
          id: "capture",
          title: "Chụp",
          body: "Kết nối máy ảnh, xem trực tiếp và để phần đếm ngược rõ ràng lo phần còn lại. Mọi ảnh đều có thể xem lại và chụp lại.",
        },
        {
          id: "create",
          title: "Sáng tạo",
          body: "Chọn khung có sẵn, tự thiết kế trong trình tạo khung, hoặc biến một tệp PNG có ô trong suốt thành khung.",
        },
        {
          id: "print",
          title: "In ảnh",
          body: "Gửi dải ảnh và ảnh đơn đến máy in Windows kèm bản xem trước vị trí, hoặc chỉ dùng ảnh kỹ thuật số mà không cần máy in.",
        },
        {
          id: "manage",
          title: "Quản lý",
          body: "Phiên chụp, ảnh gốc và ảnh ghép nằm trong thư viện cục bộ mà bạn có thể duyệt, xuất, sao lưu và khôi phục.",
        },
      ],
      more: "Xem tất cả tính năng",
    },
    journey: {
      eyebrow: "Hành trình của khách",
      title: "Sáu bước từ lời chào đến dải ảnh hoàn chỉnh.",
      steps: [
        { title: "Chọn khung", body: "Khung quyết định số ảnh sẽ được chụp." },
        { title: "Chuẩn bị", body: "Máy ảnh khởi động với hình xem trực tiếp." },
        { title: "Chụp", body: "Đếm ngược, rồi từng ảnh được chụp." },
        { title: "Xem lại và chụp lại", body: "Giữ ảnh ưng ý, chụp lại những ảnh khác." },
        { title: "Ghép ảnh", body: "Ảnh được đặt vào khung đã chọn." },
        { title: "In hoặc xuất", body: "In ra giấy, hoặc giữ ở dạng kỹ thuật số." },
      ],
    },
    hardware: {
      eyebrow: "Máy ảnh và máy in",
      title: "Trung thực về phần cứng.",
      lede: "Các tích hợp phần mềm đã có và được kiểm thử với thiết bị mô phỏng. Khả năng tương thích thực tế là một câu hỏi riêng, và chúng tôi nói rõ điều đó.",
      noneVerified:
        "Dự án chưa kiểm chứng thực tế bất kỳ mẫu máy ảnh hay máy in nào. Kết quả sẽ được công bố tại đây khi các bài thử nghiệm phần cứng hoàn tất.",
      rows: [
        {
          name: "Máy ảnh Canon EOS qua Canon EDSDK",
          status: "integration",
          detail:
            "Xem trực tiếp, bấm chụp từ xa và nhận ảnh JPEG độ phân giải đầy đủ đã được lập trình cho các máy ảnh được EDSDK đi kèm hỗ trợ. EOS R50 là máy tham chiếu; hoạt động thực tế của nó chưa được kiểm chứng.",
        },
        {
          name: "Webcam trên Windows",
          status: "integration",
          detail:
            "Webcam và thiết bị ghi hình được đọc qua FFmpeg tích hợp sẵn (DirectShow). Ảnh là khung hình lấy từ luồng video ở độ phân giải của luồng, không phải ảnh gốc từ máy ảnh. Thiết bị thật chưa được kiểm chứng.",
        },
        {
          name: "Máy in Windows",
          status: "integration",
          detail:
            "Việc in dùng bộ đệm in của Windows và trình điều khiển đã cài, với khổ giấy và vùng in đọc từ trình điều khiển. Bộ đệm in không thể chứng minh giấy đã được in ra.",
        },
        {
          name: "Máy ảnh, máy in và màn hình cảm ứng thật",
          status: "testing",
          detail:
            "EOS R50, webcam thật, bản in trên giấy và kiosk cảm ứng đều có danh sách kiểm tra phần cứng bằng văn bản, nhưng vẫn cần được chạy trên thiết bị đích.",
        },
      ],
      guide: "Đọc danh sách kiểm tra phần cứng",
    },
    local: {
      eyebrow: "Lưu trữ cục bộ",
      title: "Ảnh của bạn ở lại trên máy tính của bạn.",
      body: "Ứng dụng lưu phiên chụp, ảnh gốc, ảnh ghép và cài đặt ngay trên máy đang chạy. Không cần tài khoản, máy chủ hay kết nối internet để thực hiện một phiên chụp.",
      points: [
        {
          title: "Thư viện do bạn kiểm soát",
          body: "Mọi thứ nằm trong một thư mục trong Documents mà bạn có thể mở, sao lưu và di chuyển.",
        },
        {
          title: "Hoạt động ngoại tuyến",
          body: "Chụp, ghép, in và duyệt ảnh đều chạy trên máy cục bộ.",
        },
        {
          title: "Không cần đăng nhập",
          body: "Không tài khoản, không lưu trữ đám mây. Khi cần hỗ trợ, có thể xuất thông tin chẩn đoán thành một tệp.",
        },
      ],
      note: "Bạn vẫn chịu trách nhiệm về cách lưu trữ, chia sẻ và xóa ảnh của khách.",
    },
    downloadCta: {
      eyebrow: "Tải xuống",
      title: "Tải cho Windows.",
      body: "Trang tải xuống hiển thị bản phát hành chính thức hiện tại, khi đã có, cùng số phiên bản, dung lượng, ghi chú phát hành và mã kiểm tra SHA-256.",
      primary: "Đến trang tải xuống",
    },
  },
  features: {
    title: "Tính năng — ApexVoid Photobooth",
    description:
      "Những gì ApexVoid Photobooth làm được hôm nay: phiên chụp cho khách, máy ảnh, khung ảnh, in ấn, thư viện cục bộ và công cụ cho người vận hành — kèm ghi chú trung thực về phần cứng.",
    eyebrow: "Tính năng",
    heading: "ApexVoid Photobooth làm được gì hôm nay.",
    lede: "Danh sách này mô tả phần mềm hiện tại. Tình trạng phần cứng được mô tả riêng, vì một tích hợp phần mềm không đồng nghĩa với một thiết bị đã được kiểm chứng.",
    sections: [
      {
        id: "sessions",
        title: "Phiên chụp cho khách",
        items: [
          "Chế độ Booth toàn màn hình cho màn hình cảm ứng, dọc hoặc ngang",
          "Chọn khung trước khi máy ảnh khởi động; khung quyết định số ảnh",
          "Xem trực tiếp, đếm ngược và tiến độ từng ảnh",
          "Xem lại và chụp lại từng ảnh",
          "Khôi phục nhẹ nhàng khi máy ảnh mất kết nối: các ảnh đã chụp được giữ lại",
          "Khách có thể chuyển giữa tiếng Anh và tiếng Việt",
        ],
      },
      {
        id: "cameras",
        title: "Máy ảnh",
        items: [
          "Tích hợp Canon EDSDK cho máy ảnh EOS tương thích (tham chiếu: EOS R50)",
          "Webcam và thiết bị ghi hình trên Windows qua FFmpeg tích hợp sẵn",
          "Máy ảnh mô phỏng được ghi nhãn rõ ràng để thử nghiệm khi không có phần cứng",
          "Tự động kết nối lại và máy ảnh ưu tiên cho từng chế độ",
        ],
      },
      {
        id: "frames",
        title: "Khung và ghép ảnh",
        items: [
          "Bố cục dải ảnh, tờ ảnh và ảnh đơn có sẵn, kèm tùy chọn màu",
          "Trình tạo khung với ô ảnh, lớp chữ và lớp hình ảnh",
          "Nhập tệp PNG có ô trong suốt thành khung dùng ngay",
          "Ghép ảnh ở 300 hoặc 600 dpi",
        ],
      },
      {
        id: "printing",
        title: "In ấn",
        items: [
          "Máy in Windows qua bộ đệm in, với khổ giấy của chính trình điều khiển",
          "Bản xem trước cho thấy vị trí ảnh và vùng có thể in",
          "Hai dải ảnh 2×6 trên một tờ 4×6",
          "Lịch sử in, in lại có kiểm soát và chế độ chỉ dùng ảnh kỹ thuật số",
        ],
      },
      {
        id: "library",
        title: "Thư viện ảnh",
        items: [
          "Siêu dữ liệu SQLite và tệp cục bộ trong Documents\\apexvoid-studio",
          "Duyệt phiên chụp với tìm kiếm và bộ lọc",
          "Xuất phiên chụp thành ZIP, thùng rác có thể khôi phục",
          "Hướng dẫn sao lưu và khôi phục toàn bộ thư viện",
        ],
      },
      {
        id: "operator",
        title: "Công cụ cho người vận hành",
        items: [
          "Kiểm tra trạng thái sẵn sàng trước khi khách bắt đầu",
          "Chẩn đoán và gói hỗ trợ ngoại tuyến không chứa ảnh hay đường dẫn cá nhân",
          "Studio bằng tiếng Anh hoặc tiếng Việt, độc lập với ngôn ngữ của khách",
          "Mã PIN tùy chọn để thoát Chế độ Booth",
        ],
      },
    ],
    notYet: {
      title: "Chưa có",
      items: ["Phiên bản cho macOS và Linux", "Quay phim trực tiếp trên máy ảnh qua Canon SDK", "Tệp chạy có chữ ký số"],
    },
    gallery: {
      title: "Từ ứng dụng",
      frames: "Bộ sưu tập khung trong Studio, có trình tạo khung và nhập PNG.",
      gallery: "Thư viện trong Studio: các phiên chụp cục bộ có bộ lọc.",
      printer: "Thiết lập máy in trong Studio: Windows báo trạng thái; không mặc định là giấy đã in ra.",
      review: "Xem lại ở Chế độ Booth: ảnh nào cũng có thể chụp lại.",
    },
  },
  download: {
    title: "Tải xuống — ApexVoid Photobooth",
    description:
      "Tải ApexVoid Photobooth cho Windows 10 và 11 (x64): bản phát hành chính thức hiện tại cùng mã kiểm tra SHA-256, ghi chú phát hành và thông báo bên thứ ba.",
    eyebrow: "Tải xuống",
    heading: "Tải cho Windows",
    loading: "Đang kiểm tra bản phát hành hiện tại…",
    emptyTitle: "Chưa có bản phát hành chính thức nào.",
    emptyBody:
      "Trang này chưa có phiên bản, tệp hay mã kiểm tra nào để tải. Bản phát hành hiện tại sẽ xuất hiện tại đây ngay khi được công bố.",
    unavailableTitle: "Hiện không tải được thông tin phát hành.",
    unavailableBody:
      "Không thể tải danh sách phát hành. Hãy kiểm tra kết nối và thử lại; trang sẽ không đưa ra tệp tải nào cho đến khi tải được danh sách.",
    invalidTitle: "Không xác minh được thông tin phát hành.",
    invalidBody:
      "Danh sách phát hành đã công bố không vượt qua kiểm tra của trang, nên không có tệp nào được đưa ra để tải. Vui lòng thử lại sau hoặc báo trên GitHub.",
    retry: "Thử lại",
    noscript: "Bản phát hành hiện tại được tải bằng JavaScript. Nếu không dùng JavaScript, hãy mở trực tiếp danh sách phát hành:",
    manifest: "danh sách phát hành (JSON)",
    watch: "Theo dõi kho mã trên GitHub",
    devTitle: "Bản dựng phát triển",
    devBody:
      "Mỗi lần nhánh master được dựng thành công sẽ tạo ra một tệp chạy di động chưa ký số dưới dạng artifact của CI. Các bản này dùng để thử nghiệm; chúng không phải bản phát hành chính thức và chưa qua kiểm định phần cứng.",
    devLink: "Mở các bản dựng CI",
    requirements: "Yêu cầu hệ thống",
    req: [
      "Windows 10 hoặc Windows 11, 64-bit (x64)",
      "Microsoft Edge WebView2 Evergreen Runtime",
      "Một máy ảnh, hoặc máy ảnh mô phỏng có sẵn để thử nghiệm",
      "Trình điều khiển máy in Windows nếu bạn muốn in (không bắt buộc)",
    ],
    notSupported: "Không hỗ trợ macOS và Linux.",
    latest: "Bản phát hành mới nhất",
    previewTitle: "Bản xem trước",
    previewBody:
      "Chưa có bản phát hành ổn định nào. Bản xem trước này dùng để thử nghiệm: có thể còn tính năng chưa hoàn thiện và chưa qua kiểm định phần cứng.",
    version: "Phiên bản",
    published: "Ngày phát hành",
    file: "Tệp",
    size: "Dung lượng",
    checksum: "SHA-256",
    notes: "Ghi chú phát hành",
    checksums: "SHA256SUMS",
    notices: "Thông báo bên thứ ba",
    unsignedTitle: "Chưa ký số",
    unsignedBody:
      "Tệp chạy này chưa có chữ ký số, nên Windows SmartScreen có thể cảnh báo rằng nhà phát hành không xác định. Chỉ tải tệp từ trang này và kiểm tra mã SHA-256 trước khi chạy.",
    signedBody: "Tệp chạy có chữ ký số Authenticode.",
    canonIncluded: "Bao gồm thư viện Canon EDSDK dùng cho máy ảnh Canon.",
    canonNotIncluded: "Không bao gồm Canon EDSDK. Máy ảnh Canon cần cài đặt SDK riêng; xem hướng dẫn cài đặt.",
    verifyTitle: "Kiểm tra tệp đã tải",
    verifyBody: "Trong PowerShell, tại thư mục chứa tệp đã tải, chạy lệnh sau và so sánh kết quả với mã SHA-256 ở trên:",
  },
  releases: {
    title: "Phiên bản — ApexVoid Photobooth",
    description:
      "Lịch sử phát hành của ApexVoid Photobooth cho Windows: mọi bản phát hành chính thức cùng số phiên bản, ngày, ghi chú, mã kiểm tra và tệp tải.",
    eyebrow: "Phiên bản",
    heading: "Lịch sử phát hành",
    loading: "Đang tải lịch sử phát hành…",
    empty: "Chưa có bản phát hành chính thức nào.",
    emptyBody: "Mỗi bản phát hành chính thức sẽ được liệt kê tại đây cùng số phiên bản, ngày, ghi chú phát hành, mã kiểm tra và tệp tải.",
    unavailable: "Hiện không tải được lịch sử phát hành.",
    unavailableBody: "Không thể tải danh sách phát hành. Hãy kiểm tra kết nối và thử lại.",
    invalid: "Không xác minh được lịch sử phát hành.",
    invalidBody: "Danh sách phát hành đã công bố không vượt qua kiểm tra của trang, nên không hiển thị nội dung nào từ đó.",
    channels: "Cách phát hành hoạt động",
    channelItems: [
      {
        title: "Ổn định",
        body: "Bản phát hành chính thức được dựng từ nhánh master, nơi chứa ứng dụng desktop ổn định. Bản ổn định mới nhất là bản được đưa ra để tải.",
      },
      {
        title: "Xem trước",
        body: "Các phiên bản trước phát hành để thử nghiệm. Chúng được ghi rõ là bản xem trước và không bao giờ thay thế bản ổn định hiện tại.",
      },
      {
        title: "Phát triển",
        body: "Tính năng mới được phát triển trên nhánh community. Bản dựng từ nhánh này chỉ để thử nghiệm và không bao giờ được công bố tại đây.",
      },
    ],
    github: "Mã nguồn trên GitHub",
    prerelease: "Xem trước",
    current: "Hiện tại",
    download: "Tải xuống",
  },
  docs: {
    title: "Tài liệu — ApexVoid Photobooth",
    description:
      "Hướng dẫn cài đặt, cấu hình và thử nghiệm ApexVoid Photobooth: cài đặt, máy ảnh, in ấn, khung ảnh, kiểm tra phần cứng và kiến trúc.",
    eyebrow: "Tài liệu",
    heading: "Hướng dẫn cho người vận hành và người thử nghiệm.",
    lede: "Tài liệu hiện nằm cùng mã nguồn ứng dụng trên GitHub và sẽ được chuyển sang trang web này ở giai đoạn sau.",
    guides: [
      { id: "INSTALL", title: "Cài đặt và khôi phục", body: "Bản di động, điều kiện cần, sao lưu, khôi phục và xử lý sự cố." },
      { id: "CAMERAS", title: "Máy ảnh", body: "Thiết lập Canon và webcam, những gì được hỗ trợ và các giới hạn đã biết." },
      { id: "PRINTING", title: "In ấn", body: "Thiết lập máy in, căn chỉnh, trạng thái bộ đệm in và in lại." },
      { id: "FRAMES", title: "Khung ảnh", body: "Bố cục khung, trình tạo khung và hình ảnh tùy chỉnh." },
      { id: "HARDWARE_TESTS", title: "Kiểm tra phần cứng", body: "Quy trình kiểm tra thiết bị, màn hình cảm ứng, hệ thống tệp và độ bền." },
      { id: "ARCHITECTURE", title: "Kiến trúc", body: "Các thành phần của ứng dụng và quyết định thiết kế." },
      { id: "THIRD-PARTY", title: "Thông báo bên thứ ba", body: "Các thành phần tích hợp và giấy phép của chúng." },
    ],
    languageNote: "Các hướng dẫn hiện được viết bằng tiếng Anh.",
  },
  community: {
    title: "Cộng đồng — ApexVoid Photobooth",
    description: "Cách theo dõi ApexVoid Photobooth, báo lỗi và tham gia phát triển trên GitHub.",
    eyebrow: "Cộng đồng",
    heading: "Cùng nhau xây dựng.",
    lede: "Dự án được phát triển công khai trên GitHub. Dự án còn non trẻ, nên sự giúp đỡ giá trị nhất lúc này là thử nghiệm cẩn thận và báo cáo rõ ràng.",
    ways: [
      {
        id: "issues",
        title: "Báo lỗi",
        body: "Mô tả bạn đã làm gì, bạn mong đợi điều gì và điều gì đã xảy ra. Thông tin phần cứng và gói hỗ trợ chẩn đoán giúp ích nhiều nhất.",
        link: "Mở GitHub Issues",
      },
      {
        id: "test",
        title: "Thử nghiệm trên phần cứng thật",
        body: "Chạy danh sách kiểm tra phần cứng với máy ảnh, máy in hoặc màn hình cảm ứng của bạn và chia sẻ kết quả.",
        link: "Đọc danh sách kiểm tra phần cứng",
      },
      {
        id: "follow",
        title: "Theo dõi quá trình phát triển",
        body: "Theo dõi kho mã để thấy các thay đổi và bản phát hành đầu tiên.",
        link: "Xem kho mã",
      },
    ],
    branches: {
      title: "Cách tổ chức kho mã",
      items: [
        { name: "master", body: "Ứng dụng desktop ổn định và các bản phát hành chính thức." },
        { name: "community", body: "Phát triển tính năng cho ứng dụng desktop." },
        { name: "webpage", body: "Trang web này, được phát triển và xuất bản riêng." },
      ],
    },
    licenseNote:
      "Mã nguồn có thể xem công khai, nhưng ứng dụng chưa công bố giấy phép nào. Hãy đọc trang giấy phép trước khi sử dụng lại bất kỳ phần nào.",
  },
  privacy: {
    title: "Quyền riêng tư — ApexVoid Photobooth",
    description: "Trang web này và ứng dụng desktop ApexVoid Photobooth làm gì và không làm gì với dữ liệu.",
    eyebrow: "Quyền riêng tư",
    heading: "Thông tin về quyền riêng tư",
    updated: "Cập nhật lần cuối",
    sections: [
      {
        title: "Trang web này",
        paragraphs: [
          "Đây là một trang web tĩnh. Trang không dùng công cụ phân tích, quảng cáo, mã theo dõi hay cookie, và không tải tài nguyên từ trang web khác — phông chữ và hình ảnh được phục vụ từ chính trang này.",
          "Lựa chọn giao diện màu và ngôn ngữ của bạn được lưu trong bộ nhớ cục bộ của trình duyệt trên thiết bị của bạn để trang ghi nhớ. Thông tin này không được gửi đi đâu. Bạn có thể xóa nó bằng cách xóa dữ liệu của trang này trong trình duyệt.",
          "Máy chủ web phân phối các trang này có thể lưu nhật ký truy cập tiêu chuẩn (ví dụ địa chỉ IP, thời gian và trang được yêu cầu) phục vụ vận hành và bảo mật. Chi tiết sẽ được bổ sung khi việc lưu trữ trang được thiết lập.",
        ],
      },
      {
        title: "Ứng dụng desktop",
        paragraphs: [
          "Ứng dụng lưu phiên chụp, ảnh, ảnh ghép và cài đặt trên máy tính đang chạy. Theo tài liệu của ứng dụng, nó không cần kết nối mạng, và máy chủ xem trước chỉ lắng nghe trên máy cục bộ.",
          "Gói hỗ trợ tùy chọn là một tệp do bạn tự xuất. Gói chứa báo cáo trạng thái sẵn sàng và số lượng theo loại sự kiện nhật ký — không chứa ảnh, cài đặt, đường dẫn thư mục, tên thiết bị hay mã PIN — và ứng dụng không tải gói này lên đâu cả.",
          "Nếu bạn dùng ứng dụng tại sự kiện, bạn quyết định cách lưu giữ, chia sẻ và xóa ảnh của khách, đồng thời chịu trách nhiệm thông báo cho khách và xin sự đồng ý khi tình huống của bạn yêu cầu.",
        ],
      },
      {
        title: "Câu hỏi",
        paragraphs: ["Bạn có thể đặt câu hỏi về quyền riêng tư bằng cách tạo một issue trên GitHub."],
      },
    ],
  },
  licenses: {
    title: "Giấy phép — ApexVoid Photobooth",
    description: "Thông tin giấy phép của ứng dụng ApexVoid Photobooth, các thành phần bên thứ ba và trang web này.",
    eyebrow: "Giấy phép",
    heading: "Thông tin giấy phép",
    app: {
      title: "Ứng dụng",
      paragraphs: [
        "Ứng dụng ApexVoid Photobooth chưa công bố giấy phép nào. Mã nguồn có thể đọc trên GitHub, nhưng điều đó không cho phép sao chép, chỉnh sửa hay phân phối lại.",
        "Trang này sẽ được cập nhật khi giấy phép được lựa chọn.",
      ],
    },
    thirdParty: {
      title: "Thành phần trong ứng dụng",
      intro: "Bản dựng di động cho Windows chứa hoặc sử dụng các thành phần sau, theo mô tả trong thông báo bên thứ ba của ứng dụng:",
      items: [
        {
          name: "FFmpeg",
          license: "GNU LGPL v3",
          body: "Một bản dựng LGPL nguyên bản chạy như một tiến trình riêng, có thể thay thế, cho webcam và video.",
        },
        {
          name: "Canon EDSDK",
          license: "Độc quyền — Canon",
          body: "SDK của Canon được dùng cho máy ảnh EOS và chịu sự điều chỉnh của điều khoản riêng của Canon.",
        },
        {
          name: "Noto Sans",
          license: "SIL Open Font License 1.1",
          body: "Phông chữ dự phòng cho chữ trên khung, bao gồm ký tự tiếng Việt.",
        },
      ],
      notices: "Thông báo bên thứ ba đầy đủ",
    },
    site: {
      title: "Trang web này",
      intro: "Trang web được xây dựng bằng phần mềm mã nguồn mở:",
      items: [
        { name: "Astro", license: "MIT" },
        { name: "Tailwind CSS", license: "MIT" },
        { name: "Biểu tượng Lucide", license: "ISC" },
        { name: "Phông chữ Inter", license: "SIL Open Font License 1.1" },
        { name: "Phông chữ Fraunces", license: "SIL Open Font License 1.1" },
      ],
    },
    trademarks:
      "Canon và EOS là nhãn hiệu của Canon Inc. Windows và WebView2 là nhãn hiệu của Microsoft. FFmpeg là nhãn hiệu của Fabrice Bellard. Các tên này chỉ được dùng để mô tả khả năng tương thích.",
  },
  notFound: {
    title: "Không tìm thấy trang — ApexVoid Photobooth",
    description: "Trang này không tồn tại.",
    heading: "Trang này không nằm trong khung hình.",
    body: "Địa chỉ có thể bị gõ sai, hoặc trang đã được chuyển đi.",
    home: "Về trang chủ",
  },
  footer: {
    blurb: "Phần mềm photobooth lưu trữ cục bộ cho Windows.",
    product: "Sản phẩm",
    project: "Dự án",
    legal: "Pháp lý",
    privacy: "Quyền riêng tư",
    licenses: "Giấy phép",
    copyright: "ApexVoid. Bảo lưu mọi quyền.",
    unofficial: "Không liên kết với Canon hay Microsoft.",
  },
};
