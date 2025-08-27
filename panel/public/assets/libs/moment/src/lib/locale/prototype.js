import {Locale} from "./constructor";
import {calendar} from "./calendar";
import {longDateFormat} from "./formats";
import {invalidDate} from "./invalid";
import {ordinal} from "./ordinal";
import {preParsePostFormat} from "./pre-post-format";
import {pastFuture, relativeTime} from "./relative";
import {set} from "./set";
// Month
import {localeMonths, localeMonthsParse, localeMonthsShort, monthsRegex, monthsShortRegex,} from "../units/month";
// Week
import {localeFirstDayOfWeek, localeFirstDayOfYear, localeWeek,} from "../units/week";
// Day of Week
import {
    localeWeekdays,
    localeWeekdaysMin,
    localeWeekdaysParse,
    localeWeekdaysShort,
    weekdaysMinRegex,
    weekdaysRegex,
    weekdaysShortRegex,
} from "../units/day-of-week";
// Hours
import {localeIsPM, localeMeridiem} from "../units/hour";

var proto = Locale.prototype;

proto.calendar = calendar;
proto.longDateFormat = longDateFormat;
proto.invalidDate = invalidDate;
proto.ordinal = ordinal;
proto.preparse = preParsePostFormat;
proto.postformat = preParsePostFormat;
proto.relativeTime = relativeTime;
proto.pastFuture = pastFuture;
proto.set = set;

proto.months = localeMonths;
proto.monthsShort = localeMonthsShort;
proto.monthsParse = localeMonthsParse;
proto.monthsRegex = monthsRegex;
proto.monthsShortRegex = monthsShortRegex;

proto.week = localeWeek;
proto.firstDayOfYear = localeFirstDayOfYear;
proto.firstDayOfWeek = localeFirstDayOfWeek;

proto.weekdays = localeWeekdays;
proto.weekdaysMin = localeWeekdaysMin;
proto.weekdaysShort = localeWeekdaysShort;
proto.weekdaysParse = localeWeekdaysParse;

proto.weekdaysRegex = weekdaysRegex;
proto.weekdaysShortRegex = weekdaysShortRegex;
proto.weekdaysMinRegex = weekdaysMinRegex;

proto.isPM = localeIsPM;
proto.meridiem = localeMeridiem;
