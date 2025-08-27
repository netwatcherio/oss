export interface Remote {
    connecting: boolean,
    connected: boolean,

    connect(): void,

    disconnect(): void,
}
