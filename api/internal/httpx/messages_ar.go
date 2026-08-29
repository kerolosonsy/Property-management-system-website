package httpx

// Arabic user-facing messages. All messages state what the user should do next
// (FR-027) and never expose internal detail (FR-028).

const (
	// Generic / input
	MsgInternalError      = "حدث خطأ غير متوقع. الرجاء المحاولة مرة أخرى."
	MsgInvalidRequest     = "الطلب غير صالح. تأكد من البيانات المدخلة."
	MsgInvalidJSON        = "صيغة الطلب غير صحيحة."
	MsgUnauthorized       = "يلزم تسجيل الدخول للمتابعة."
	MsgForbidden          = "ليست لديك صلاحية لتنفيذ هذا الإجراء."
	MsgNotFound           = "العنصر المطلوب غير موجود."
	MsgConflict           = "تعذّر تنفيذ الإجراء بسبب تعارض في البيانات."

	// Auth
	MsgInvalidCredentials      = "اسم المستخدم أو كلمة المرور غير صحيحة."
	MsgTooSoon                 = "الانتظار قبل المحاولة مرة أخرى."
	MsgAccountDeactivated      = "اسم المستخدم أو كلمة المرور غير صحيحة." // identical to invalid_credentials (FR-013)
	MsgPasswordChangeRequired  = "يجب تغيير كلمة المرور قبل المتابعة."
	MsgPasswordTooShort        = "يجب ألا تقل كلمة المرور عن 12 حرفًا."
	MsgPasswordWrongCurrent    = "كلمة المرور الحالية غير صحيحة."
	MsgPasswordChanged         = "تم تغيير كلمة المرور بنجاح."
	MsgSignedIn                = "تم تسجيل الدخول بنجاح."
	MsgSignedOut               = "تم تسجيل الخروج بنجاح."

	// Username validation (FR-036, FR-037, FR-038, FR-039)
	MsgUsernameLength     = "يجب أن يكون اسم المستخدم بين 3 و 32 حرفًا."
	MsgUsernameChars      = "يجب أن يحتوي اسم المستخدم على حروف عربية أو لاتينية أو أرقام أو نقطة أو شرطة سفلية أو شرطة."
	MsgUsernameHasInvisible = "يحتوي اسم المستخدم على أحرف غير مرئية أو أحرف للتحكم في الاتجاه. الرجاء إزالتها."
	MsgUsernameTaken      = "اسم المستخدم مستخدم بالفعل. اختر اسمًا آخر."

	// Display name
	MsgDisplayNameLength = "يجب أن يكون اسم العرض بين 1 و 120 حرفًا."

	// Account administration
	MsgLastAdmin        = "لا يمكن تنفيذ هذا الإجراء؛ يجب أن يبقى مدير نشط واحد على الأقل في النظام."
	MsgSelfTarget       = "لا يمكنك تنفيذ هذا الإجراء على حسابك الشخصي."
	MsgRoleRequiredAdmin = "هذا الإجراء متاح للمدير فقط."

	// Sessions
	MsgSessionExpired   = "انتهت صلاحية الجلسة. الرجاء تسجيل الدخول مرة أخرى."
	MsgSessionMissing   = "يلزم تسجيل الدخول للمتابعة."

	// Audit records / filters
	MsgInvalidDateRange  = "تاريخ البداية يجب أن يكون قبل تاريخ النهاية."
	MsgNoRecords         = "لا توجد سجلات مطابقة. يمكنك تعديل عوامل التصفية."
)
