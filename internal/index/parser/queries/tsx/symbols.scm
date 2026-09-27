(function_declaration name: (identifier) @name) @function
(class_declaration name: (type_identifier) @name) @class
(method_definition name: (property_identifier) @name) @method
(generator_function_declaration name: (identifier) @name) @function
(variable_declarator name: (identifier) @name value: (arrow_function)) @function
(variable_declarator name: (identifier) @name value: (function_expression)) @function
(function_signature name: (identifier) @name) @function
(method_signature name: (property_identifier) @name) @method
(abstract_method_signature name: (property_identifier) @name) @method
