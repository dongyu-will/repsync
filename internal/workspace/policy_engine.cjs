// Uses the target project's flat ESLint configuration and parser, including TS/TSX.
const fs = require('node:fs');
const path = require('node:path');
const { createRequire } = require('node:module');

const restriction = {
  meta: { type: 'problem', schema: [], messages: { forbidden: '禁止使用 React useState；请使用 useReducer。' } },
  create(context) {
    const source = context.sourceCode;
    const reported = new WeakSet();
    const property = (node) => node.computed ? node.property?.value : node.property?.name;
    const isRequire = (node) => node?.type === 'CallExpression' && node.callee?.name === 'require' && node.arguments[0]?.value === 'react';
    function variable(node) {
      for (let scope = source.getScope(node); scope; scope = scope.upper) {
        const found = scope.set.get(node.name);
        if (found) return found;
      }
    }
    function namespace(node, seen = new Set()) {
      if (isRequire(node)) return true;
      if (node?.type !== 'Identifier') return false;
      const binding = variable(node);
      if (!binding || seen.has(binding)) return false;
      seen.add(binding);
      return binding.defs.some((def) => {
        if (def.type === 'ImportBinding') return def.parent?.source?.value === 'react' && ['ImportDefaultSpecifier', 'ImportNamespaceSpecifier'].includes(def.node.type);
        return def.type === 'Variable' && def.node.id.type === 'Identifier' && namespace(def.node.init, seen);
      });
    }
    function stateBinding(node, seen = new Set()) {
      const binding = variable(node);
      if (!binding || seen.has(binding)) return false;
      seen.add(binding);
      return binding.defs.some((def) => {
        if (def.type === 'ImportBinding') return def.parent?.source?.value === 'react' && def.node.type === 'ImportSpecifier' && (def.node.imported.name || def.node.imported.value) === 'useState';
        if (def.type !== 'Variable') return false;
        if (def.node.id.type === 'Identifier') {
          const init=def.node.init;
          return init?.type==='Identifier' ? stateBinding(init,seen) : init?.type==='MemberExpression' && property(init)==='useState' && namespace(init.object);
        }
        if (def.node.id.type !== 'ObjectPattern' || !namespace(def.node.init)) return false;
        return def.node.id.properties.some((p) => (p.key?.name || p.key?.value) === 'useState' && (p.value?.name === node.name || p.value?.left?.name === node.name));
      });
    }
    const report = (node) => { if (!reported.has(node)) { reported.add(node); context.report({node, messageId: 'forbidden'}); } };
    return {
      ImportDeclaration(node) {
        if (node.source.value !== 'react') return;
        for (const specifier of node.specifiers) if (specifier.type === 'ImportSpecifier' && (specifier.imported.name || specifier.imported.value) === 'useState') report(specifier);
      },
      ExportNamedDeclaration(node) {
        if (node.source?.value !== 'react') return;
        for (const specifier of node.specifiers) if ((specifier.local?.name || specifier.local?.value) === 'useState') report(specifier);
      },
      Identifier(node) {
        if (!stateBinding(node)) return;
        const binding = variable(node);
        if (binding.references.some((reference) => reference.identifier === node)) report(node);
      },
      MemberExpression(node) { if (property(node) === 'useState' && namespace(node.object)) report(node); },
      VariableDeclarator(node) {
        if (node.id.type !== 'ObjectPattern' || !namespace(node.init)) return;
        for (const property of node.id.properties) if ((property.key?.name || property.key?.value) === 'useState') report(property);
      },
    };
  },
};

(async () => {
  const input = JSON.parse(fs.readFileSync(0, 'utf8'));
  const localRequire = createRequire(path.join(input.root, 'package.json'));
  const api = localRequire('eslint');
  if (!api.loadESLint) throw new Error('需要项目已有的 ESLint 9+ 和 flat config');
  const ESLint = await api.loadESLint({useFlatConfig:true});
  const eslint = new ESLint({cwd: input.root, overrideConfig: {plugins: {'repsync-policy': {rules: {'no-use-state':restriction}}}, rules: {'repsync-policy/no-use-state':'error'}}});
  const output = {findings:[], errors:[], skipped:[]};
  for (const file of input.files) {
    if (await eslint.isPathIgnored(path.join(input.projectRoot,file.path))) {output.skipped.push(file.path);continue;}
    const results = await eslint.lintText(file.content, {filePath:path.join(input.projectRoot,file.path), warnIgnored:true});
    for (const result of results) {
      for (const message of result.messages) {
        if (message.ruleId === 'repsync-policy/no-use-state') output.findings.push({path:file.path, line:message.line, column:message.column, message:message.message});
        else if (message.fatal || message.ruleId === null) output.errors.push(`${file.path}: ${message.message}`);
      }
    }
    if (output.findings.length >= 500) { output.errors.push('违规超过 500 条，检查结果不完整'); break; }
  }
  process.stdout.write(JSON.stringify(output));
})().catch((error) => { process.stderr.write(`React 规则检查失败：${error.message}`); process.exitCode = 1; });
