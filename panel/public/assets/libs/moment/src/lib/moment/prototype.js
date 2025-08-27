import {Moment} from "./constructor";
import {add, subtract} from "./add-subtract";
import {calendar} from "./calendar";
import {clone} from "./clone";
import {isAfter, isBefore, isBetween, isSame, isSameOrAfter, isSameOrBefore,} from "./compare";
import {diff} from "./diff";
import {format, inspect, toISOString, toString} from "./format";
import {from, fromNow} from "./from";
import {to, toNow} from "./to";
import {stringGet, stringSet} from "./get-set";
import {lang, locale, localeData} from "./locale";
import {prototypeMax, prototypeMin} from "./min-max";
import {endOf, startOf} from "./start-end-of";
import {toArray, toDate, toJSON, toObject, unix, valueOf} from "./to-type";
import {invalidAt, isValid, parsingFlags} from "./valid";
import {creationData} from "./creation-data";
// Year
import {getIsLeapYear, getSetYear} from "../units/year";
// Week Year
import {getISOWeeksInYear, getSetISOWeekYear, getSetWeekYear, getWeeksInYear,} from "../units/week-year";
// Quarter
import {getSetQuarter} from "../units/quarter";
// Month
import {getDaysInMonth, getSetMonth} from "../units/month";
// Week
import {getSetISOWeek, getSetWeek} from "../units/week";
// Day
import {getSetDayOfMonth} from "../units/day-of-month";
import {getSetDayOfWeek, getSetISODayOfWeek, getSetLocaleDayOfWeek,} from "../units/day-of-week";
import {getSetDayOfYear} from "../units/day-of-year";
// Hour
import {getSetHour} from "../units/hour";
// Minute
import {getSetMinute} from "../units/minute";
// Second
import {getSetSecond} from "../units/second";
// Millisecond
import {getSetMillisecond} from "../units/millisecond";
// Offset
import {
    getSetOffset,
    getSetZone,
    hasAlignedHourOffset,
    isDaylightSavingTime,
    isDaylightSavingTimeShifted,
    isLocal,
    isUtc,
    isUtcOffset,
    setOffsetToLocal,
    setOffsetToParsedOffset,
    setOffsetToUTC,
} from "../units/offset";
// Timezone
import {getZoneAbbr, getZoneName} from "../units/timezone";
// Deprecations
import {deprecate} from "../utils/deprecate";

var proto = Moment.prototype;

proto.add = add;
proto.calendar = calendar;
proto.clone = clone;
proto.diff = diff;
proto.endOf = endOf;
proto.format = format;
proto.from = from;
proto.fromNow = fromNow;
proto.to = to;
proto.toNow = toNow;
proto.get = stringGet;
proto.invalidAt = invalidAt;
proto.isAfter = isAfter;
proto.isBefore = isBefore;
proto.isBetween = isBetween;
proto.isSame = isSame;
proto.isSameOrAfter = isSameOrAfter;
proto.isSameOrBefore = isSameOrBefore;
proto.isValid = isValid;
proto.lang = lang;
proto.locale = locale;
proto.localeData = localeData;
proto.max = prototypeMax;
proto.min = prototypeMin;
proto.parsingFlags = parsingFlags;
proto.set = stringSet;
proto.startOf = startOf;
proto.subtract = subtract;
proto.toArray = toArray;
proto.toObject = toObject;
proto.toDate = toDate;
proto.toISOString = toISOString;
proto.inspect = inspect;
proto.toJSON = toJSON;
proto.toString = toString;
proto.unix = unix;
proto.valueOf = valueOf;
proto.creationData = creationData;

proto.year = getSetYear;
proto.isLeapYear = getIsLeapYear;

proto.weekYear = getSetWeekYear;
proto.isoWeekYear = getSetISOWeekYear;

proto.quarter = proto.quarters = getSetQuarter;

proto.month = getSetMonth;
proto.daysInMonth = getDaysInMonth;

proto.week = proto.weeks = getSetWeek;
proto.isoWeek = proto.isoWeeks = getSetISOWeek;
proto.weeksInYear = getWeeksInYear;
proto.isoWeeksInYear = getISOWeeksInYear;

proto.date = getSetDayOfMonth;
proto.day = proto.days = getSetDayOfWeek;
proto.weekday = getSetLocaleDayOfWeek;
proto.isoWeekday = getSetISODayOfWeek;
proto.dayOfYear = getSetDayOfYear;

proto.hour = proto.hours = getSetHour;

proto.minute = proto.minutes = getSetMinute;

proto.second = proto.seconds = getSetSecond;

proto.millisecond = proto.milliseconds = getSetMillisecond;

proto.utcOffset = getSetOffset;
proto.utc = setOffsetToUTC;
proto.local = setOffsetToLocal;
proto.parseZone = setOffsetToParsedOffset;
proto.hasAlignedHourOffset = hasAlignedHourOffset;
proto.isDST = isDaylightSavingTime;
proto.isLocal = isLocal;
proto.isUtcOffset = isUtcOffset;
proto.isUtc = isUtc;
proto.isUTC = isUtc;

proto.zoneAbbr = getZoneAbbr;
proto.zoneName = getZoneName;

proto.dates = deprecate(
    "dates accessor is deprecated. Use date instead.",
    getSetDayOfMonth
);
proto.months = deprecate(
    "months accessor is deprecated. Use month instead",
    getSetMonth
);
proto.years = deprecate(
    "years accessor is deprecated. Use year instead",
    getSetYear
);
proto.zone = deprecate(
    "moment().zone is deprecated, use moment().utcOffset instead. http://momentjs.com/guides/#/warnings/zone/",
    getSetZone
);
proto.isDSTShifted = deprecate(
    "isDSTShifted is deprecated. See http://momentjs.com/guides/#/warnings/dst-shifted/ for more information",
    isDaylightSavingTimeShifted
);

export default proto;
