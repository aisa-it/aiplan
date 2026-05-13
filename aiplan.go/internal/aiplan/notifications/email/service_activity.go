package email

func (es *EmailService) EmailActivity() {
	if es.cfg.EmailActivityDisabled {
		return
	}

	if es.sending {
		return
	}

	templates := LoadTemplates(es.db)
	ProcessLayer(es, NewSprintPipeline(&templates), templates)
}
