; Python guard queries: test functions, assertions, markers, comments.
; Tree-sitter nodes only — strings and comments never match assert or
; decorator patterns structurally (decision D-174).

(function_definition
  name: (identifier) @test.name
  body: (_) @test.body) @test.func

(decorator) @marker

(assert_statement) @assert

(comment) @comment
