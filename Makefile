



README.md: *.go
	echo "**Lakefile**" > $@
	echo "\`\`\`hcl" >> $@
	cat Lakefile >> $@
	echo "\`\`\`" >> $@
	echo "\`\`\`json" >> $@
	go run . >> $@
	echo "\`\`\`" >> $@
