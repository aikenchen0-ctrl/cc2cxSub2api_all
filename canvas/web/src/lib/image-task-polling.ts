// Image storage is optional; the original task URL remains usable on failure.
export async function imageTaskStorageWithinDeadline<T>(storage: Promise<T>, timeoutMs = 3000): Promise<T | null> {
    let timer: ReturnType<typeof setTimeout> | undefined;
    try {
        return await Promise.race([
            storage.catch(() => null),
            new Promise<null>((resolve) => { timer = setTimeout(() => resolve(null), timeoutMs); }),
        ]);
    } finally {
        clearTimeout(timer);
    }
}
