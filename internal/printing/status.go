package printing

import "strings"

// Windows spooler job status flags (JOB_STATUS_*).
const (
	JobPaused           = 0x1
	JobError            = 0x2
	JobDeleting         = 0x4
	JobSpooling         = 0x8
	JobPrinting         = 0x10
	JobOffline          = 0x20
	JobPaperOut         = 0x40
	JobPrinted          = 0x80
	JobDeleted          = 0x100
	JobBlockedDevQ      = 0x200
	JobUserIntervention = 0x400
	JobRestart          = 0x800
	JobComplete         = 0x1000
	JobRetained         = 0x2000
)

// Windows printer status flags (PRINTER_STATUS_*) and attributes.
const (
	PrinterPaused          = 0x1
	PrinterError           = 0x2
	PrinterPendingDelete   = 0x4
	PrinterPaperJam        = 0x8
	PrinterPaperOut        = 0x10
	PrinterManualFeed      = 0x20
	PrinterPaperProblem    = 0x40
	PrinterOffline         = 0x80
	PrinterOutputBinFull   = 0x800
	PrinterNotAvailable    = 0x1000
	PrinterNoToner         = 0x40000
	PrinterDoorOpen        = 0x400000
	PrinterUserIntervent   = 0x100000
	PrinterAttrWorkOffline = 0x400
)

// MapSpoolerJob converts spooler flags into a UI status. present=false means
// the job is no longer in the queue. The wording never claims that paper was
// delivered.
func MapSpoolerJob(flags uint32, present bool) JobState {
	if !present {
		return JobState{Status: StatusRemoved, Detail: "The job left the print queue. It was most likely printed, but the spooler gives no proof that paper came out."}
	}
	var problems []string
	add := func(f uint32, s string) {
		if flags&f != 0 {
			problems = append(problems, s)
		}
	}
	add(JobError, "the printer reported an error")
	add(JobOffline, "the printer is offline")
	add(JobPaperOut, "out of paper")
	add(JobBlockedDevQ, "blocked: the printer cannot accept the job")
	add(JobUserIntervention, "the printer needs attention")
	if len(problems) > 0 {
		return JobState{Status: StatusFailed, Detail: "Not printing: " + strings.Join(problems, ", ") + ". Fix it at the printer, or cancel the job in the Windows print queue."}
	}
	switch {
	case flags&(JobDeleted|JobDeleting) != 0:
		return JobState{Status: StatusFailed, Detail: "The job was cancelled/deleted in the print queue."}
	case flags&(JobPrinted|JobComplete) != 0:
		return JobState{Status: StatusCompleted, Detail: "The spooler reports the job as printed (this is not proof that paper came out)."}
	case flags&JobPrinting != 0:
		return JobState{Status: StatusPrinting, Detail: "Printing."}
	case flags&JobPaused != 0:
		return JobState{Status: StatusQueued, Detail: "The job is paused in the queue."}
	case flags&JobSpooling != 0:
		return JobState{Status: StatusQueued, Detail: "Spooling."}
	}
	return JobState{Status: StatusQueued, Detail: "Waiting in the print queue."}
}

// MapPrinterStatus converts PRINTER_INFO_2 status/attributes into availability.
func MapPrinterStatus(status, attributes uint32) (state, detail string) {
	var p []string
	add := func(f uint32, s string) {
		if status&f != 0 {
			p = append(p, s)
		}
	}
	add(PrinterPaperJam, "paper jam")
	add(PrinterPaperOut, "out of paper")
	add(PrinterPaperProblem, "paper problem")
	add(PrinterNoToner, "out of ink/toner")
	add(PrinterDoorOpen, "door open")
	add(PrinterOutputBinFull, "output tray full")
	add(PrinterUserIntervent, "needs user attention")
	add(PrinterError, "error")
	switch {
	case attributes&PrinterAttrWorkOffline != 0 || status&(PrinterOffline|PrinterNotAvailable) != 0:
		return "offline", "The printer is offline or unavailable. Switch it on, check the cable/network, and clear “Use Printer Offline” in Windows."
	case status&PrinterPendingDelete != 0:
		return "error", "The printer is being removed."
	case len(p) > 0:
		return "error", strings.Join(p, ", ")
	case status&PrinterPaused != 0:
		return "paused", "The Windows printer queue is paused. Resume it in Windows before printing."
	}
	return "ready", ""
}
