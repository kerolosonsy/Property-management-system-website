package httpx

// Arabic user-facing messages. All messages state what the user should do next
// and never expose internal detail, a key, a wrapped key, or a decrypted
// value (Constitution VII).

const (
	// Generic / input
	MsgInternalError      = "حدث خطأ غير متوقع. الرجاء المحاولة مرة أخرى."
	MsgInvalidRequest     = "الطلب غير صالح. تأكد من البيانات المدخلة."
	MsgInvalidJSON        = "صيغة الطلب غير صحيحة."
	MsgUnauthorized       = "يلزم تسجيل الدخول للمتابعة."
	MsgForbidden          = "ليست لديك صلاحية لتنفيذ هذا الإجراء."
	MsgNotFound           = "العنصر المطلوب غير موجود."
	MsgConflict           = "تعذّر تنفيذ الإجراء بسبب تعارض في البيانات."
	MsgInUse              = "العنصر مستخدم. تعذّرت إزالته."
	MsgVersionConflict    = "تغيّرت بيانات هذا العنصر منذ فتحه. الرجاء إعادة فتحه وحفظ التعديلات من جديد."
	MsgArchived           = "هذا العنصر مؤرشف. أعد تنشيطه قبل التعديل."

	// Auth
	MsgInvalidCredentials      = "اسم المستخدم أو كلمة المرور غير صحيحة."
	MsgTooSoon                 = "الانتظار قبل المحاولة مرة أخرى."
	MsgAccountDeactivated      = "اسم المستخدم أو كلمة المرور غير صحيحة." // identical to invalid_credentials
	MsgPasswordChangeRequired  = "يجب تغيير كلمة المرور قبل المتابعة."
	MsgPasswordTooShort        = "يجب ألا تقل كلمة المرور عن 12 حرفًا."
	MsgPasswordWrongCurrent    = "كلمة المرور الحالية غير صحيحة."
	MsgPasswordChanged         = "تم تغيير كلمة المرور بنجاح."
	MsgSignedIn                = "تم تسجيل الدخول بنجاح."
	MsgSignedOut               = "تم تسجيل الخروج بنجاح."

	// Username validation
	MsgUsernameLength       = "يجب أن يكون اسم المستخدم بين 3 و 32 حرفًا."
	MsgUsernameChars        = "يجب أن يحتوي اسم المستخدم على حروف عربية أو لاتينية أو أرقام أو نقطة أو شرطة سفلية أو شرطة."
	MsgUsernameHasInvisible = "يحتوي اسم المستخدم على أحرف غير مرئية أو أحرف للتحكم في الاتجاه. الرجاء إزالتها."
	MsgUsernameTaken        = "اسم المستخدم مستخدم بالفعل. اختر اسمًا آخر."

	// Display name
	MsgDisplayNameLength = "يجب أن يكون اسم العرض بين 1 و 120 حرفًا."

	// Account administration
	MsgLastAdmin         = "لا يمكن تنفيذ هذا الإجراء؛ يجب أن يبقى مدير نشط واحد على الأقل في النظام."
	MsgSelfTarget        = "لا يمكنك تنفيذ هذا الإجراء على حسابك الشخصي."
	MsgRoleRequiredAdmin = "هذا الإجراء متاح للمدير فقط."

	// Sessions
	MsgSessionExpired = "انتهت صلاحية الجلسة. الرجاء تسجيل الدخول مرة أخرى."
	MsgSessionMissing = "يلزم تسجيل الدخول للمتابعة."

	// Audit records / filters
	MsgInvalidDateRange = "تاريخ البداية يجب أن يكون قبل تاريخ النهاية."
	MsgNoRecords        = "لا توجد سجلات مطابقة. يمكنك تعديل عوامل التصفية."

	// Properties
	MsgPropertyNameLength    = "يجب أن يكون اسم العقار بين 2 و 120 حرفًا."
	MsgPropertyNameRequired  = "اسم العقار مطلوب."
	MsgPropertyTypeRequired  = "يجب اختيار نوع العقار."
	MsgPropertyAreaRequired  = "يجب اختيار المنطقة."
	MsgPropertyCodeLength    = "يجب أن يكون الرمز المرجعي بين 1 و 32 حرفًا."
	MsgPropertyCodeTaken     = "الرمز المرجعي مستخدم بالفعل لعقار آخر، نشط أو مؤرشف."
	MsgPropertyCodeArchived  = "الرمز المرجعي يخص عقارًا مؤرشفًا. أعد تنشيطه بدلًا من إنشاء عقار جديد."
	MsgPropertyNotFound      = "العقار المطلوب غير موجود."
	MsgPropertyArchivedRead  = "هذا العقار مؤرشف. اقرأه أو أعد تنشيطه."
	MsgPropertyArchivedTitle = "أرشفة العقار"
	MsgPropertyRestoreTitle  = "إعادة تنشيط العقار"
	MsgPropertyArchivedQ     = "هل تريد أرشفة العقار «{name}»؟ سيُحذف من قائمة العمل ويمكن استرجاعه لاحقًا."
	MsgPropertyRestoreQ      = "هل تريد إعادة تنشيط العقار «{name}»؟ سيعود إلى قائمة العمل."
	MsgPropertyVersionNeeded = "تم تعديل هذا العقار منذ فتحه. الرجاء إعادة فتحه قبل الحفظ."
	MsgPropertyEmptyRegister = "لا توجد عقارات بعد. أضف أول عقار من زر «إضافة عقار»."
	MsgPropertyNoMatch       = "لا توجد عقارات مطابقة للبحث والتصفية الحاليين."
	MsgPropertyArchivedNone  = "لا توجد عقارات مؤرشفة."
	MsgPropertyArchiveNoteLength = "يجب ألّا يتجاوز سبب الأرشفة 500 حرفًا."
	MsgLookupLabelLength     = "يجب أن يكون الاسم بين 1 و 60 حرفًا."
	MsgLookupLabelTaken      = "هذا الاسم مستخدم بالفعل في القائمة."
	MsgLookupInUseSingular   = "العنصر مستخدم في عقار واحد ({count}). تعذّرت إزالته."
	MsgLookupInUsePlural     = "العنصر مستخدم في {count} عقار. تعذّرت إزالته."
	MsgLookupNotFound        = "العنصر المطلوب غير موجود في القائمة."

	// Custom fields
	MsgCustomFieldLabelLength       = "يجب أن يكون اسم الحقل بين 1 و 60 حرفًا."
	MsgCustomFieldLabelTaken        = "اسم الحقل مستخدم بالفعل."
	MsgCustomFieldSensitiveTextOnly = "لا يجوز وسم حقل اختيار أو متعدد الاختيارات بأنه سري. الأنواع المسموح بها للنصوص الحرة فقط."
	MsgCustomFieldChoicesRequired   = "يجب إضافة اختيار واحد على الأقل لحقل الاختيار."
	MsgCustomFieldNotFound          = "تعريف الحقل غير موجود."
	MsgCustomFieldInUseSingular     = "الحقل مستخدم في عقار واحد ({count}). تعذّر تغييره أو حذفه."
	MsgCustomFieldInUsePlural       = "الحقل مستخدم في {count} عقار. تعذّر تغييره أو حذفه."
	MsgCustomFieldChoiceInUse       = "الاختيار مستخدم في بعض العقارات. تعذّرت إزالته."
	MsgCustomFieldValueSize         = "قيمة الحقل طويلة جدًا."
	MsgCustomFieldSensitiveNote     = "تذكير: القيم التي تخص الهوية الوطنية أو جواز السفر أو الحساب البنكي أو رقم الـ IBAN يجب أن تُسجّل في حقل نصي سرّي. تركها في حقل عادي يخالف سياسة التشفير المعتمدة."
	MsgCustomFieldSensitiveSearch   = "الحقول السرية لا تظهر في البحث المتقدم ولا يمكن تصفية النتائج بها."

	// Advanced search
	MsgSearchUnknownField  = "أحد الحقول المخصصة في البحث غير معروف. الرجاء إعادة فتح الشاشة."
	MsgSearchSensitiveField = "لا يجوز البحث في الحقول السرية. احذفها من عوامل التصفية."
	MsgSearchOperatorMismatch = "عامل التصفية لا يناسب نوع الحقل."
	MsgSearchInvalidValue  = "قيمة التصفية غير صالحة."

	// Attachments (feature 003-property-attachments-ocr)
	MsgAttachmentDescriptionLength     = "يجب أن يكون وصف المرفق بين 2 و 200 حرفًا."
	MsgAttachmentTooLarge              = "حجم الملف يتجاوز الحد المسموح به ({limit} بايت)."
	MsgAttachmentUnsupportedType       = "نوع الملف غير مدعوم. الأنواع المسموح بها: PDF، صور (PNG، JPG، JPEG، GIF، BMP، TIFF)، ومستندات أوفيس الحديثة (DOCX، XLSX، PPTX)."
	MsgAttachmentKeyUnavailable        = "تعذّر الوصول إلى مفتاح التشفير. لا يمكن تخزين الملف. أعد تشغيل الخدمة بعد التحقق من الإعدادات."
	MsgAttachmentIntegrityFailed       = "تعذّر قراءة الملف من وحدة التخزين؛ يبدو أنه تالف أو معدّل. لم يُسلَّم أي جزء منه."
	MsgAttachmentArchivedRefused       = "العقار مؤرشف. أعد تنشيطه لإضافة مرفقات أو تعديلها."
	MsgAttachmentDemotionRefused       = "لا يمكن إرجاع المرفق من حال السري إلى حال عادي بعد ترقيته."
	MsgAttachmentMissingDescription    = "وصف المرفق مطلوب."
	MsgAttachmentNotFound              = "المرفق المطلوب غير موجود."
	MsgAttachmentReextractCorrected    = "تم تصحيح نص هذا المرفق يدويًا. لا يمكن إعادة الاستخراج لأن التصحيح سيُفقَد. امسح التصحيح أولاً إذا أردت المحاولة مرة أخرى."
	MsgAttachmentReextractRunning      = "استخراج النص جارٍ لهذا المرفق. الرجاء الانتظار حتى يكتمل."
	MsgAttachmentReextractInvalidState = "لا يمكن إعادة الاستخراج من هذه الحالة. المسموح من «فشل» أو «لم يُعثر على نص» فقط."

	// Extraction state labels (Arabic; the database enum values stay English for
	// cross-tool compatibility and the contract).
	MsgExtractStatePending     = "قيد المعالجة"
	MsgExtractStateDone        = "تم بنجاح"
	MsgExtractStateEmpty       = "لم يُعثر على نص"
	MsgExtractStateNotEligible = "غير قابل للاستخراج"
	MsgExtractStateTooLarge    = "النص طويل جدًا للمعالجة"
	MsgExtractStateFailed      = "فشل الاستخراج"
	MsgExtractTruncated        = "النص مقطوع لتجاوز الحد المسموح"

	// Extraction reason codes (mirrored in the database). These are reason
	// codes, never file content (FR-033).
	MsgExtractReasonTesseractMissing      = "أداة التعرف على النص (tesseract) غير مثبّتة"
	MsgExtractReasonTesseractNoArabic     = "حزمة اللغة العربية لـ tesseract غير مثبّتة"
	MsgExtractReasonPopplerMissing        = "أدوات قراءة PDF (poppler) غير مثبّتة"
	MsgExtractReasonLegacyOffice          = "صيغة Office القديمة غير مدعومة للاستخراج النصي"
	MsgExtractReasonUnsupportedType       = "نوع الملف غير مدعوم للاستخراج"
	MsgExtractReasonTextTooLarge          = "النص المستخرج أطول من الحد المسموح"
	MsgExtractReasonUnknown               = "سبب غير معروف"

	// Document search
	MsgDocSearchSensitiveExcluded = "الملفات السرية لا تظهر في البحث ولا يمكن البحث في نصّها."

	// Undo (item 5, audit before/after + undo system). Arabic, user-facing.
	MsgAuditUnrestorable         = "تعذّر التراجع: الحالة الأصلية لهذا التغيير لم تُحفظ."
	MsgAuditVersionMoved        = "تعذّر التراجع: تغيّرت بيانات هذا العنصر بعد تنفيذ التغيير. أعد فتحه وحاول التراجع من جديد."
	MsgAuditRestoreCollision    = "تعذّر التراجع: العنصر القديم الذي كان هذا التغيير يستعيده محجوز الآن من قِبل سجلّ آخر."
	MsgAuditAlreadyUndone       = "هذا السجل تمّ التراجع عنه من قبل."
	MsgAuditCannotUndoUndo      = "لا يمكن التراجع عن سجلّ التراجع نفسه."
	MsgAuditSensitiveUnrestorable = "تعذّر التراجع: التغيير الأصلي شمل حقلًا سرّيًا لا يمكن استعادة قيمته."
	MsgAuditAlreadyActive       = "العقار نشط فعلًا. لا يمكن التراجع عن أرشفته."
	MsgAuditAlreadyArchived     = "العقار مؤرشف فعلًا. لا يمكن التراجع عن إعادة تنشيطه."
)
