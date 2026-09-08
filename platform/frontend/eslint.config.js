/* ESLint acotado a UN objetivo: cazar identificadores que NO existen.
 *
 * POR QUÉ EXISTE ESTE ARCHIVO. Tres veces en un día el módulo Restaurante llegó a
 * producción en BLANCO por la misma causa: un refactor borró un helper (`T`,
 * `totalCuenta`, `ITEM_COLOR`) que otro lugar seguía usando. React no renderiza nada ante
 * un ReferenceError, así que el usuario ve una pantalla vacía sin explicación.
 *
 * Y nada lo detectaba: `vite build` compila sin chistar un identificador inexistente
 * (solo resuelve imports, no referencias), y las pruebas de Vitest cubren `src/lib`
 * —lógica pura—, no las pantallas. El único control era abrir cada pestaña a mano.
 *
 * Deliberadamente NO es una configuración de estilo: no opina sobre comillas, orden de
 * imports ni hooks. Un linter que grita por 300 cosas se ignora, y entonces tampoco
 * avisa de la que importa. Solo reglas que atrapan código ROTO.
 */
import js from '@eslint/js'
import globals from 'globals'
import reactPlugin from 'eslint-plugin-react'
import reactHooks from 'eslint-plugin-react-hooks'

export default [
  { ignores: ['dist/**', 'node_modules/**', 'public/**'] },
  {
    files: ['src/**/*.{js,jsx}'],
    languageOptions: {
      ecmaVersion: 2022,
      sourceType: 'module',
      globals: { ...globals.browser, ...globals.es2021 },
      parserOptions: { ecmaFeatures: { jsx: true } },
    },
    // react-hooks se registra con TODAS sus reglas apagadas: no se quiere su opinión,
    // pero el código tiene comentarios `eslint-disable react-hooks/exhaustive-deps` y sin
    // el plugin ESLint falla con «rule not found». Registrarlo los hace resolver.
    plugins: { react: reactPlugin, 'react-hooks': reactHooks },
    // Los `eslint-disable react-hooks/*` del código quedan sin efecto (esas reglas están
    // apagadas) y ESLint los reportaría como directivas inútiles. Es ruido: los
    // comentarios documentan una decisión y se conservan.
    linterOptions: { reportUnusedDisableDirectives: 'off' },
    settings: { react: { version: '18.3' } },
    rules: {
      // EL núcleo: una variable, función o constante usada y no declarada.
      'no-undef': 'error',
      // Su equivalente en JSX: <Componente /> que no existe (un import olvidado).
      'react/jsx-no-undef': 'error',
      // NOTA: se evaluó `no-use-before-define` y se DESCARTÓ. Daba 15 falsos positivos
      // sobre un patrón legítimo y extendido en este código: una función flecha declarada
      // más abajo y referenciada dentro de un useCallback/useEffect de más arriba, que
      // solo se EJECUTA después (el TDZ no se viola). Dejarla convertiría el linter en
      // ruido, y un linter ruidoso se ignora — y entonces tampoco avisa de lo que importa.
      // Devolver algo dentro de un constructor, `case` que se cae al siguiente, etc.
      'no-dupe-keys': 'error',
      'no-dupe-args': 'error',
      'no-dupe-class-members': 'error',
      // Una condición constante suele ser un `&&` mal cerrado.
      'no-constant-condition': ['error', { checkLoops: false }],
      'no-unreachable': 'error',
      // `case` sin break que cae al siguiente: casi siempre un olvido.
      'no-fallthrough': 'error',
    },
  },
  {
    // Las pruebas corren en Node y usan los globals de Vitest.
    files: ['src/**/*.{test,spec}.{js,jsx}'],
    languageOptions: { globals: { ...globals.node, ...globals.vitest } },
  },
]
