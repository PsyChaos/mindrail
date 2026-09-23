; TypeScript/JavaScript guard queries: test calls, expectations, comments.
; Node types only — string contents never match structurally.

(call_expression
  function: (_) @call.func
  arguments: (_) @call.args) @call

(comment) @comment
