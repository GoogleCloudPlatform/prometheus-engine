export declare enum ValueType {
    none = "none",
    vector = "vector",
    scalar = "scalar",
    matrix = "matrix",
    string = "string"
}
export interface PromQLFunction {
    name: string;
    argTypes: ValueType[];
    variadic: number;
    returnType: ValueType;
}
export declare function getFunction(id: number): PromQLFunction;
