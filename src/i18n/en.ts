/**
 * English copy — the source dictionary. Every other locale must provide exactly this shape (see `Dict`),
 * so a missing or extra key is a type error, not an untranslated string on the page.
 *
 * Product statements here are checked against the desktop application's documentation and confirmed test results.
 * Do not name a device model as tested unless that exact model has been confirmed.
 */
export const en = {
  meta: {
    siteName: "ApexVoid Photobooth",
    tagline: "Local-first photobooth software for Windows",
  },
  a11y: {
    skip: "Skip to content",
    mainNav: "Main",
    footerNav: "Footer",
    openMenu: "Open menu",
    closeMenu: "Close menu",
    theme: "Colour theme",
    themeLight: "Light",
    themeDark: "Dark",
    themeSystem: "System",
    language: "Language",
    switchTo: "Read this page in English",
    newTab: "(opens in a new tab)",
    home: "ApexVoid Photobooth home",
  },
  nav: {
    features: "Features",
    download: "Download",
    docs: "Documentation",
    community: "Community",
    releases: "Releases",
    github: "GitHub",
  },
  common: {
    downloadWindows: "Download for Windows",
    exploreFeatures: "Explore features",
    viewOnGithub: "View on GitHub",
    openGuide: "Open the guide",
    screenshotNote: "Screenshots show the desktop application's interface, captured in its browser test harness.",
    platform: {
      os: "Windows 10 and 11",
      arch: "x64",
      portable: "Portable — one executable",
      webview: "Needs the Microsoft Edge WebView2 Runtime",
    },
  },
  home: {
    title: "ApexVoid Photobooth — local-first photobooth software for Windows",
    description:
      "A modern, local-first photobooth application for Windows: guided touchscreen sessions, custom frames, Windows printing and a media library that stays on your computer.",
    hero: {
      eyebrow: "Photobooth software for Windows",
      title: "Capture moments.",
      titleAccent: "Create memories.",
      lede: "A modern, local-first photobooth application designed for creators, studios, events and unforgettable experiences.",
      imageAlt: "Booth Mode frame selection: a grid of photo-strip designs with one selected and a large preview beside it.",
    },
    experience: {
      eyebrow: "Two experiences, one application",
      title: "A calm stage for guests. A clear desk for the operator.",
      booth: {
        label: "Booth Mode",
        title: "The guest takes the lead.",
        body: "A fullscreen, touch-first journey: guests pick a frame first, see themselves live, count down, review every photo, retake the ones they don't like and leave with a composed strip.",
        points: [
          "Frame selection before the camera starts",
          "Live preview, a large countdown and capture feedback",
          "Review with individual retakes",
          "Composition in the chosen frame, ready to print",
        ],
        imageAlt: "Booth Mode countdown: a large number 3 over the live preview, with the chosen frame beside it.",
        secondAlt: "Booth Mode result: the finished photo strip with Print, Change design and Finish buttons.",
      },
      studio: {
        label: "Studio Mode",
        title: "Everything the operator needs, before the guests arrive.",
        body: "Set up cameras and printers, manage frames, browse and export sessions, and check readiness — with the guest screen kept separate and protected by an optional exit PIN.",
        points: [
          "Camera, printer and readiness checks",
          "Frame catalogue and frame builder",
          "Local media library with search and export",
          "Diagnostics and an offline support bundle",
        ],
        imageAlt: "Studio dashboard with camera, printer and storage status and recent sessions.",
      },
    },
    groups: {
      eyebrow: "What it does",
      title: "Built around the moment, not the menu.",
      items: [
        {
          id: "capture",
          title: "Capture",
          body: "Connect a camera, see a live preview and let a clear countdown do the rest. Every photo can be reviewed and retaken.",
        },
        {
          id: "create",
          title: "Create",
          body: "Choose from built-in frames, design your own in the frame builder, or turn a PNG with transparent windows into a frame.",
        },
        {
          id: "print",
          title: "Print",
          body: "Send strips and photos to a Windows printer with a placement preview, or run digital-only with no printer at all.",
        },
        {
          id: "manage",
          title: "Manage",
          body: "Sessions, originals and compositions stay in a local library you can browse, export, back up and restore.",
        },
      ],
      more: "See all features",
    },
    journey: {
      eyebrow: "The guest journey",
      title: "Six steps from hello to a finished strip.",
      steps: [
        { title: "Choose a frame", body: "The frame decides how many photos are taken." },
        { title: "Get ready", body: "The camera starts with a live preview." },
        { title: "Capture", body: "A countdown, then each photo is taken." },
        { title: "Review and retake", body: "Keep the favourites, retake the rest." },
        { title: "Compose", body: "Photos are placed into the chosen frame." },
        { title: "Print or export", body: "Print it, or keep it digital." },
      ],
    },
    hardware: {
      eyebrow: "Camera integration",
      title: "Professional camera integration.",
      lede: "Connect a camera for live preview and automated capture, then tailor each session to your event. Camera workflows have been tested with a real camera.",
      supportNote: "Choose Canon EOS through EDSDK, a Windows webcam, or the simulated camera for setup and configuration.",
      rows: [
        {
          name: "Canon EOS cameras",
          detail: "EDSDK integration provides live view, remote capture and full-resolution JPEG transfer for compatible EOS cameras.",
        },
        {
          name: "Windows webcams",
          detail: "Connect webcams and video capture devices through embedded FFmpeg and DirectShow.",
        },
        {
          name: "Live camera preview",
          detail: "See the camera feed in Booth Mode before and during a photo session.",
        },
        {
          name: "Automated photo sessions",
          detail: "Use countdown-led capture and set the number of shots independently of the selected frame.",
        },
        {
          name: "Simulated camera",
          detail: "Explore setup and configuration without connecting a physical camera.",
        },
      ],
    },
    local: {
      eyebrow: "Local-first",
      title: "Your photos stay on your computer.",
      body: "The application keeps sessions, originals, compositions and settings on the machine it runs on. It needs no account, no server and no internet connection to run a session.",
      points: [
        {
          title: "A library you control",
          body: "Everything lives in a folder in Documents that you can open, back up and move.",
        },
        {
          title: "Works offline",
          body: "Shooting, composing, printing and browsing run on the local machine.",
        },
        {
          title: "Nothing to sign in to",
          body: "No accounts and no cloud storage. Diagnostics can be exported as a file when you need help.",
        },
      ],
      note: "You remain responsible for how guest photos are stored, shared and deleted.",
    },
    downloadCta: {
      eyebrow: "Download",
      title: "Get it for Windows.",
      body: "The download page shows the current official release, when one is published, with its version, size, release notes and SHA-256 checksum.",
      primary: "Go to the download page",
    },
  },
  features: {
    title: "Features — ApexVoid Photobooth",
    description:
      "Explore guest sessions, Canon EDSDK and Windows webcam capture, custom frames, printing, a local media library and studio tools.",
    eyebrow: "Features",
    heading: "What ApexVoid Photobooth does today.",
    lede: "A clear overview of the capture, design, printing and studio tools available in ApexVoid Photobooth.",
    sections: [
      {
        id: "sessions",
        title: "Guest sessions",
        items: [
          "Fullscreen Booth Mode for touchscreens, in portrait or landscape",
          "Frame selection before the camera starts; the frame sets the photo count",
          "Live preview, countdown and per-photo progress",
          "Review with individual retakes",
          "Calm recovery when the camera disconnects: photos already taken are kept",
          "Guests can switch between English and Vietnamese",
        ],
      },
      {
        id: "cameras",
        title: "Cameras",
        items: [
          "Canon EOS integration through EDSDK for compatible cameras",
          "Windows webcam and capture-device support through embedded FFmpeg",
          "Live camera preview and countdown-led automated capture",
          "Configurable shot count independent of the selected frame",
          "A clearly labelled simulated camera for testing without hardware",
          "Automatic reconnection and a preferred camera per mode",
        ],
      },
      {
        id: "frames",
        title: "Frames and composition",
        items: [
          "Built-in strip, sheet and single-photo layouts with colour options",
          "A frame builder with photo slots, text and image layers",
          "Import a PNG with transparent windows as a ready frame",
          "Composition at 300 or 600 dpi",
        ],
      },
      {
        id: "printing",
        title: "Printing",
        items: [
          "Windows printers through the spooler, with the driver's own paper sizes",
          "A preview that shows placement and the printable area",
          "Two 2×6 strips on one 4×6 sheet",
          "Print history, guarded reprints and a digital-only mode",
        ],
      },
      {
        id: "library",
        title: "Media library",
        items: [
          "Local SQLite metadata and files in Documents\\apexvoid-studio",
          "Session browsing with search and filters",
          "Session ZIP export, recoverable trash and restore",
          "Backup and restore guidance for the whole library",
        ],
      },
      {
        id: "operator",
        title: "Operator tools",
        items: [
          "A readiness check before guests start",
          "Diagnostics and an offline support bundle without media or personal paths",
          "An English or Vietnamese Studio, independent of the guest language",
          "An optional PIN to leave Booth Mode",
        ],
      },
    ],
    notYet: {
      title: "Not available",
      items: ["macOS and Linux versions", "Camera-native movie recording through the Canon SDK", "Code-signed executables"],
    },
    gallery: {
      title: "From the application",
      frames: "Studio frame catalogue with the builder and PNG import.",
      gallery: "Studio gallery: local sessions with filters.",
      printer: "Studio printer setup with available devices, paper sizes and a print layout preview.",
      review: "Booth review: every photo can be retaken.",
    },
  },
  download: {
    title: "Download — ApexVoid Photobooth",
    description:
      "Download ApexVoid Photobooth for Windows 10 and 11 (x64): the current official release with its SHA-256 checksum, release notes and third-party notices.",
    eyebrow: "Download",
    heading: "Download for Windows",
    loading: "Checking for the current release…",
    emptyTitle: "No official release has been published yet.",
    emptyBody:
      "There is no version, file or checksum to download from this site yet. This page shows the current release as soon as one is published.",
    unavailableTitle: "Release information is unavailable right now.",
    unavailableBody:
      "The list of releases could not be loaded. Check your connection and try again; nothing is offered for download until it loads.",
    invalidTitle: "Release information could not be verified.",
    invalidBody:
      "The published release list did not pass this site's checks, so no download is offered. Please try again later or report it on GitHub.",
    retry: "Try again",
    noscript: "The current release is loaded with JavaScript. Without it, open the release list directly:",
    manifest: "release list (JSON)",
    watch: "Follow the repository on GitHub",
    devTitle: "Development builds",
    devBody:
      "Each successful build of the master branch produces an unsigned portable executable as a CI artifact. These builds are for development and are not official releases.",
    devLink: "Open the CI builds",
    requirements: "System requirements",
    req: [
      "Windows 10 or Windows 11, 64-bit (x64)",
      "Microsoft Edge WebView2 Evergreen Runtime",
      "A camera, or the built-in simulated camera for testing",
      "A Windows printer driver if you want to print (optional)",
    ],
    notSupported: "macOS and Linux are not supported.",
    latest: "Current release",
    archiveTitle: "Previous versions",
    archiveDescription: "Only the four most recent previous versions are retained. We recommend using the latest version.",
    archiveEmpty: "No previous versions",
    archiveEmptyBody: "There are no earlier stable releases in the archive yet.",
    version: "Version",
    published: "Published",
    file: "File",
    size: "Size",
    checksum: "SHA-256",
    notes: "Release notes",
    checksums: "SHA256SUMS",
    notices: "Third-party notices",
    unsignedTitle: "Not code-signed",
    unsignedBody:
      "This executable has no digital signature yet, so Windows SmartScreen may warn that the publisher is unknown. Download it only from this site and check its SHA-256 before running it.",
    signedBody: "The executable carries an Authenticode digital signature.",
    canonIncluded: "Includes the Canon EDSDK runtime used for Canon cameras.",
    canonNotIncluded: "Does not include the Canon EDSDK. Canon cameras need the SDK set up separately; see the installation guide.",
    verifyTitle: "Check the download",
    verifyBody: "In PowerShell, in the folder you downloaded to, run this and compare the result with the SHA-256 above:",
  },
  releases: {
    title: "Releases — ApexVoid Photobooth",
    description: "Current and archived stable versions of ApexVoid Photobooth for Windows, with release notes, checksums and downloads.",
    eyebrow: "Releases",
    heading: "Release history",
    loading: "Loading the release history…",
    empty: "No official release has been published yet.",
    emptyBody: "Each official release will be listed here with its version, date, release notes, checksum and download.",
    unavailable: "The release history is unavailable right now.",
    unavailableBody: "The list of releases could not be loaded. Check your connection and try again.",
    invalid: "The release history could not be verified.",
    invalidBody: "The published release list did not pass this site's checks, so nothing from it is shown.",
    channels: "How releases work",
    channelItems: [
      {
        title: "Stable",
        body: "Official stable releases are built from the master branch. The release manifest selects the current version, including after an operator rollback.",
      },
      {
        title: "Archive",
        body: "The current stable release and up to four previous stable versions are listed. Older public release files are removed automatically.",
      },
      {
        title: "Development",
        body: "Feature work happens on the community branch. Its builds are for testing only and are never published here.",
      },
    ],
    github: "Source code on GitHub",
    current: "Current",
    download: "Download",
  },
  docs: {
    title: "Documentation — ApexVoid Photobooth",
    description:
      "Guides for installing, configuring and testing ApexVoid Photobooth: installation, cameras, printing, frames, hardware tests and architecture.",
    eyebrow: "Documentation",
    heading: "Guides for operators and testers.",
    lede: "The documentation currently lives with the application source on GitHub. It will move to this site in a later phase.",
    guides: [
      { id: "INSTALL", title: "Install and recovery", body: "Portable build, prerequisites, backup, restore and troubleshooting." },
      { id: "CAMERAS", title: "Cameras", body: "Canon and webcam setup, what is supported and the known limits." },
      { id: "PRINTING", title: "Printing", body: "Printer setup, calibration, spooler status and reprints." },
      { id: "FRAMES", title: "Frames", body: "Frame layouts, the frame builder and custom artwork." },
      { id: "HARDWARE_TESTS", title: "Hardware tests", body: "Device, touchscreen, filesystem and endurance test procedures." },
      { id: "ARCHITECTURE", title: "Architecture", body: "Application components and design decisions." },
      { id: "THIRD-PARTY", title: "Third-party notices", body: "Embedded components and their licences." },
    ],
    languageNote: "The guides are written in English.",
  },
  community: {
    title: "Community — ApexVoid Photobooth",
    description: "How to follow ApexVoid Photobooth, report problems and take part in its development on GitHub.",
    eyebrow: "Community",
    heading: "Build it with us.",
    lede: "Development happens in the open on GitHub. The project is young, so the most valuable help today is careful testing and clear reports.",
    ways: [
      {
        id: "issues",
        title: "Report a problem",
        body: "Describe what you did, what you expected and what happened. Hardware details and the diagnostics support bundle help most.",
        link: "Open GitHub Issues",
      },
      {
        id: "test",
        title: "Share your setup",
        body: "Share your setup and help improve compatibility notes for different cameras, printers and touchscreens.",
        link: "Read the hardware checklist",
      },
      {
        id: "follow",
        title: "Follow development",
        body: "Watch the repository to see changes and the first release.",
        link: "View the repository",
      },
    ],
    branches: {
      title: "How the repository is organised",
      items: [
        { name: "master", body: "The stable desktop application and official releases." },
        { name: "community", body: "Desktop application feature development." },
        { name: "webpage", body: "This website, developed and published separately." },
      ],
    },
    licenseNote:
      "The application is open source under the MIT License. Bundled third-party components keep their own licences; see the licensing page.",
  },
  privacy: {
    title: "Privacy — ApexVoid Photobooth",
    description: "What this website and the ApexVoid Photobooth desktop application do and do not do with data.",
    eyebrow: "Privacy",
    heading: "Privacy information",
    updated: "Last updated",
    sections: [
      {
        title: "This website",
        paragraphs: [
          "This is a static website. It uses no analytics, no advertising, no tracking scripts and no cookies, and it loads no resources from other websites — fonts and images are served from this site.",
          "Your choice of colour theme and language is stored in your browser's local storage on your device, so the site can remember it. It is not sent anywhere. You can clear it by clearing this site's data in your browser.",
          "The web server that delivers these pages may keep standard access logs (for example IP address, time and requested page) for operation and security. Details will be added here when hosting is set up.",
        ],
      },
      {
        title: "The desktop application",
        paragraphs: [
          "The application stores sessions, photos, compositions and settings on the computer it runs on. According to its documentation it needs no network connection, and its preview server listens only on the local machine.",
          "The optional support bundle is a file you export yourself. It contains a readiness report and counts of log event categories — not media, settings, folder paths, device names or PINs — and the application does not upload it.",
          "If you run the application at events, you decide how guest photos are kept, shared and deleted, and you are responsible for informing guests and obtaining any consent your situation requires.",
        ],
      },
      {
        title: "Questions",
        paragraphs: ["Questions about privacy can be raised as an issue on GitHub."],
      },
    ],
  },
  licenses: {
    title: "Licensing — ApexVoid Photobooth",
    description: "Licensing information for the ApexVoid Photobooth application, its third-party components and this website.",
    eyebrow: "Licensing",
    heading: "Licensing information",
    app: {
      title: "The application",
      paragraphs: [
        "ApexVoid Photobooth is open source under the MIT License. You may use, copy, modify and redistribute it, including commercially, as long as the copyright and licence notice are kept. It comes without any warranty.",
        "The MIT License covers the application's own code. The components listed below keep their own licences; every release includes their notices (THIRD-PARTY-NOTICES.txt) and the licence text (LICENSE.txt).",
      ],
    },
    thirdParty: {
      title: "Components in the application",
      intro: "The portable Windows build contains or uses these components, as described in its third-party notices:",
      items: [
        {
          name: "FFmpeg",
          license: "GNU LGPL v3",
          body: "An unmodified LGPL build runs as a separate, replaceable process for webcams and video.",
        },
        {
          name: "Canon EDSDK",
          license: "Proprietary — Canon",
          body: "Canon's SDK is used for EOS cameras and is embedded in the official Windows build. It remains Canon's property under Canon's own terms and is not covered by the MIT License.",
        },
        {
          name: "Noto Sans",
          license: "SIL Open Font License 1.1",
          body: "A fallback font for frame text, including Vietnamese characters.",
        },
      ],
      notices: "Full third-party notices",
    },
    site: {
      title: "This website",
      intro: "The website is built with open-source software:",
      items: [
        { name: "Astro", license: "MIT" },
        { name: "Tailwind CSS", license: "MIT" },
        { name: "Lucide icons", license: "ISC" },
        { name: "Inter typeface", license: "SIL Open Font License 1.1" },
        { name: "Fraunces typeface", license: "SIL Open Font License 1.1" },
      ],
    },
    trademarks:
      "Canon and EOS are trademarks of Canon Inc. Windows and WebView2 are trademarks of Microsoft. FFmpeg is a trademark of Fabrice Bellard. They are used only to describe compatibility.",
  },
  notFound: {
    title: "Page not found — ApexVoid Photobooth",
    description: "This page does not exist.",
    heading: "This page isn't in the frame.",
    body: "The address may be mistyped, or the page may have moved.",
    home: "Back to the home page",
  },
  footer: {
    blurb: "Local-first photobooth software for Windows.",
    product: "Product",
    project: "Project",
    legal: "Legal",
    privacy: "Privacy",
    licenses: "Licensing",
    copyright: "ApexVoid. All rights reserved.",
    unofficial: "Not affiliated with Canon or Microsoft.",
  },
};

type Widen<T> = T extends string
  ? string
  : T extends readonly (infer U)[]
    ? Widen<U>[]
    : T extends object
      ? { [K in keyof T]: Widen<T[K]> }
      : T;

/** The shape every locale must provide. */
export type Dict = Widen<typeof en>;
