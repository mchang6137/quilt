# Compiles all of the docs in docs/build.
all: build/slate _jsdoc.erb
	cd build/slate && \
	cp -r ${CURDIR}/source/* source/ && \
	cp -r ${CURDIR}/*md source/ && \
	cp ${CURDIR}/_jsdoc.erb source/ && \
	bundle exec middleman build --clean && \
	cp -r build/* ${CURDIR}/build/ && \
	echo "docs successfully compiled to HTML. To view docs, run:" && \
	echo "  open build/index.html"

clean:
	rm _jsdoc.erb
	rm -rf build

# Generate the JavaScript documentation by using jsdoc and Quilt's template.
_jsdoc.erb: build/jsdoc_template ../blueprint/
	build/jsdoc_template/node_modules/.bin/jsdoc \
		-t build/jsdoc_template/ \
		$(abspath ${CURDIR}/../blueprint/bindings.js) -d ./

build/jsdoc_template:
	mkdir -p build && \
	cd build && \
	git clone https://github.com/quilt/jsdoc_template.git && \
	cd jsdoc_template && \
	npm install .

build/slate:
	mkdir -p build && \
	cd build && \
	git clone https://github.com/quilt/slate.git && \
	cd slate && \
	bundle install
