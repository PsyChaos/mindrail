; JavaScript guard queries: identical node vocabulary to TypeScript
; for the call shapes this guard reads.

(call_expression
  function: (_) @call.func
  arguments: (_) @call.args) @call

(comment) @comment
