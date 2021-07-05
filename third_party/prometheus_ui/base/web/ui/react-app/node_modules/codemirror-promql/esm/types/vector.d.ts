export declare enum VectorMatchCardinality {
    CardOneToOne = "one-to-one",
    CardManyToOne = "many-to-one",
    CardOneToMany = "one-to-many",
    CardManyToMany = "many-to-many"
}
export interface VectorMatching {
    card: VectorMatchCardinality;
    matchingLabels: string[];
    on: boolean;
    include: string[];
}
