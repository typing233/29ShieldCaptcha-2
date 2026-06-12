export interface PoWResult {
    solution: string;
    iterations: number;
}
export declare function solvePoW(id: string, nonce: string, difficulty: number, onProgress?: (iterations: number) => void): Promise<PoWResult>;
