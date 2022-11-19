

README.md: *.go
	echo "**Lakefile**" > $@
	echo "\`\`\`hcl" >> $@
	cat Lakefile >> $@
	echo "\`\`\`" >> $@
	echo "" >> $@
	echo "**Json Output:**" >> $@
	echo "" >> $@
	echo "\`\`\`json" >> $@
	go run . >> $@
	echo "\`\`\`" >> $@
