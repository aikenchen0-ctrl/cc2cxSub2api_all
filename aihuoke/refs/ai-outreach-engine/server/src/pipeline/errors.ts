/**
 * Signals "not now, but nothing is wrong": send windows, daily caps and
 * throttles. The runner reschedules without consuming a retry attempt, so a
 * queue of 200 approved messages does not exhaust its attempts overnight.
 */
export class RetryLater extends Error {
  constructor(
    message: string,
    public delaySeconds: number,
  ) {
    super(message);
    this.name = "RetryLater";
  }
}

/** Cooperative stop requested from the UI. Not a failure — do not retry. */
export class JobCancelled extends Error {
  constructor(message = "Cancelled by user") {
    super(message);
    this.name = "JobCancelled";
  }
}
