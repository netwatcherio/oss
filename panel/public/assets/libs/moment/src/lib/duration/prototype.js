import {Duration} from "./constructor";
import {abs} from "./abs";
import {add, subtract} from "./add-subtract";
import {as, asDays, asHours, asMilliseconds, asMinutes, asMonths, asSeconds, asWeeks, asYears, valueOf,} from "./as";
import {bubble} from "./bubble";
import {clone} from "./clone";
import {days, get, hours, milliseconds, minutes, months, seconds, weeks, years,} from "./get";
import {humanize} from "./humanize";
import {toISOString} from "./iso-string";
import {lang, locale, localeData} from "../moment/locale";
import {isValid} from "./valid";
// Deprecations
import {deprecate} from "../utils/deprecate";

var proto = Duration.prototype;

proto.isValid = isValid;
proto.abs = abs;
proto.add = add;
proto.subtract = subtract;
proto.as = as;
proto.asMilliseconds = asMilliseconds;
proto.asSeconds = asSeconds;
proto.asMinutes = asMinutes;
proto.asHours = asHours;
proto.asDays = asDays;
proto.asWeeks = asWeeks;
proto.asMonths = asMonths;
proto.asYears = asYears;
proto.valueOf = valueOf;
proto._bubble = bubble;
proto.clone = clone;
proto.get = get;
proto.milliseconds = milliseconds;
proto.seconds = seconds;
proto.minutes = minutes;
proto.hours = hours;
proto.days = days;
proto.weeks = weeks;
proto.months = months;
proto.years = years;
proto.humanize = humanize;
proto.toISOString = toISOString;
proto.toString = toISOString;
proto.toJSON = toISOString;
proto.locale = locale;
proto.localeData = localeData;

proto.toIsoString = deprecate(
    "toIsoString() is deprecated. Please use toISOString() instead (notice the capitals)",
    toISOString
);
proto.lang = lang;
