(function_declaration name: (identifier) @name) @function
(class_declaration name: (identifier) @name) @class
(method_definition name: (property_identifier) @name) @method
(generator_function_declaration name: (identifier) @name) @function
(variable_declarator name: (identifier) @name value: (arrow_function)) @function
(variable_declarator name: (identifier) @name value: (function_expression)) @function
