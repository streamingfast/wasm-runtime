#!/usr/bin/env bash

# Here the actual graph-cli invocation of the `asc` tool (called programatically)
#
#     asc
#     [
#         inputFile,
#         global,
#         '--baseDir',
#         baseDir,
#         '--lib',
#         libs,
#         '--outFile',
#         outputFile,
#         '--optimize',
#         '--debug',
#     ],

ROOT="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

main() {
    cd "$ROOT" > /dev/null

    graphLib="node_modules/@graphprotocol/graph-ts/global/global.ts"

    for input in `ls src/**/*`; do
        output=`printf $input | sed -E 's|^src/|build/|' | sed -E 's|\.ts$|.wasm|'`

        echo "Compiling $input => $output ..."
        yarn -s run asc "$input" "$graphLib" --baseDir "$ROOT" --lib "$ROOT/node_modules" --outFile "$output" --optimize --debug
    done

    echo "Completed"
}

main $@
