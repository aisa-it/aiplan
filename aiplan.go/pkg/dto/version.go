package dto

// EntityName — как называть сущность ядра в интерфейсе. Пустая форма —
// «оставить стандартную».
type EntityName struct {
	// One — единственное число: «Задача».
	One string `json:"one"`
	// Many — множественное число: «Задачи».
	Many string `json:"many"`
	// Genitive — родительный падеж для подписей вида «нет задачи»: «задачи».
	Genitive string `json:"genitive"`
	// Create — подпись действия создания: «Новая задача».
	Create string `json:"create"`
}

// EntityNames — названия сущностей по ключу (engine.Entity*). Отсутствующий
// ключ — стандартное название.
type EntityNames map[string]EntityName

// VersionInfo — версия сервера и флаги сборки для фронта.
type VersionInfo struct {
	Version string `json:"version"`
	SignUp  bool   `json:"sign_up"`
	Demo    bool   `json:"demo"`
	NY      bool   `json:"ny"`
	Captcha bool   `json:"captcha"`
	Jitsi   bool   `json:"jitsi"`
	// EntityNames — названия сущностей от движка.
	EntityNames EntityNames `json:"entity_names"`
}
