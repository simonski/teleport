// Parse independent browser scripts/modules in one Node process, without execution.
const fs = require('node:fs');
const vm = require('node:vm');
for (const filename of process.argv.slice(2)) {
  const source=fs.readFileSync(filename,'utf8');
  try { new vm.Script(source,{filename}); }
  catch (error) {
    // Match Node's syntax detection for .js: retry as an ES module.
    new vm.SourceTextModule(source,{identifier:filename});
  }
}
