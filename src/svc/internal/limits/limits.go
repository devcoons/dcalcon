package limits

const (
	MaxICSBytes                  = 8 << 20   // 8 MiB, matches CalDAV max-resource-size
	MaxVCardBytes                = 1 << 20   // 1 MiB
	MaxPhotoBytes                = 256 << 10 // 256 KiB per PHOTO/LOGO
	MaxHTTPBody                  = 10 << 20  // whole request
	MaxAttachmentBytes           = 8 << 20   // 8 MiB per file
	MaxAttachmentsPerObject      = 20
	MaxAttachmentsBytesPerObject = 32 << 20 // 32 MiB per event/task
	MaxAppPasswords              = 20
	MaxImportCards               = 2000
	MaxImportEvents              = 2000
	MaxImportZipFiles            = 2500
	MaxBackupZip                 = 64 << 20 // user backup upload
)
