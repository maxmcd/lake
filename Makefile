

.PHONY: format
format:
	nomad fmt lake/
	cat Lakefile | nomad fmt - > Lakefile
