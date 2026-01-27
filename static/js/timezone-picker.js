/**
 * Searchable Timezone Picker Component
 *
 * Usage:
 *   const picker = new TimezonePicker({
 *     inputId: 'timezone-search',
 *     hiddenInputId: 'timezone-value',
 *     dropdownId: 'timezone-results',
 *     onSelect: function(tz) { console.log('Selected:', tz); }
 *   });
 *   picker.init();
 *
 * The component fetches timezone data from /api/timezones and provides:
 * - Text search filtering (matches ID, display name, and common abbreviations)
 * - Dropdown with filtered results showing timezone name, current time, and UTC offset
 * - Keyboard navigation (Arrow Up/Down, Enter, Escape)
 * - Click to select
 */

// Common timezone abbreviations mapped to IANA IDs
var TIMEZONE_ABBREVIATIONS = {
  'EST': ['America/New_York'],
  'EDT': ['America/New_York'],
  'CST': ['America/Chicago', 'Asia/Shanghai'],
  'CDT': ['America/Chicago'],
  'MST': ['America/Denver', 'America/Phoenix'],
  'MDT': ['America/Denver'],
  'PST': ['America/Los_Angeles'],
  'PDT': ['America/Los_Angeles'],
  'AKST': ['America/Anchorage'],
  'AKDT': ['America/Anchorage'],
  'HST': ['Pacific/Honolulu'],
  'AST': ['America/Halifax'],
  'ADT': ['America/Halifax'],
  'NST': ['America/St_Johns'],
  'NDT': ['America/St_Johns'],
  'GMT': ['Europe/London', 'UTC', 'Europe/Dublin'],
  'BST': ['Europe/London'],
  'WET': ['Europe/Lisbon'],
  'WEST': ['Europe/Lisbon'],
  'CET': ['Europe/Paris', 'Europe/Berlin', 'Europe/Rome', 'Europe/Amsterdam', 'Europe/Brussels', 'Europe/Madrid', 'Europe/Vienna', 'Europe/Zurich', 'Europe/Stockholm', 'Europe/Oslo', 'Europe/Copenhagen', 'Europe/Warsaw', 'Europe/Prague', 'Europe/Budapest', 'Europe/Belgrade'],
  'CEST': ['Europe/Paris', 'Europe/Berlin', 'Europe/Rome', 'Europe/Amsterdam', 'Europe/Brussels', 'Europe/Madrid', 'Europe/Vienna', 'Europe/Zurich', 'Europe/Stockholm', 'Europe/Oslo', 'Europe/Copenhagen', 'Europe/Warsaw', 'Europe/Prague', 'Europe/Budapest', 'Europe/Belgrade'],
  'EET': ['Europe/Athens', 'Europe/Bucharest', 'Europe/Helsinki', 'Europe/Kiev', 'Africa/Cairo'],
  'EEST': ['Europe/Athens', 'Europe/Bucharest', 'Europe/Helsinki', 'Europe/Kiev'],
  'MSK': ['Europe/Moscow'],
  'IST': ['Asia/Kolkata', 'Asia/Jerusalem'],
  'GST': ['Asia/Dubai'],
  'PKT': ['Asia/Karachi'],
  'BDT': ['Asia/Dhaka'],
  'ICT': ['Asia/Bangkok', 'Asia/Ho_Chi_Minh'],
  'WIB': ['Asia/Jakarta'],
  'SGT': ['Asia/Singapore'],
  'MYT': ['Asia/Kuala_Lumpur'],
  'PHT': ['Asia/Manila'],
  'HKT': ['Asia/Hong_Kong'],
  'JST': ['Asia/Tokyo'],
  'KST': ['Asia/Seoul'],
  'AEST': ['Australia/Sydney', 'Australia/Melbourne', 'Australia/Brisbane'],
  'AEDT': ['Australia/Sydney', 'Australia/Melbourne'],
  'ACST': ['Australia/Adelaide', 'Australia/Darwin'],
  'ACDT': ['Australia/Adelaide'],
  'AWST': ['Australia/Perth'],
  'NZST': ['Pacific/Auckland'],
  'NZDT': ['Pacific/Auckland'],
  'UTC': ['UTC']
};

function TimezonePicker(options) {
  this.inputId = options.inputId;
  this.hiddenInputId = options.hiddenInputId;
  this.dropdownId = options.dropdownId;
  this.onSelect = options.onSelect || function () { };
  this.initialValue = options.initialValue || '';

  this.timezones = [];
  this.filteredTimezones = [];
  this.selectedIndex = -1;
  this.isOpen = false;

  this.inputEl = null;
  this.hiddenInputEl = null;
  this.dropdownEl = null;
}

TimezonePicker.prototype.init = function () {
  var self = this;

  this.inputEl = document.getElementById(this.inputId);
  this.hiddenInputEl = document.getElementById(this.hiddenInputId);
  this.dropdownEl = document.getElementById(this.dropdownId);

  if (!this.inputEl || !this.dropdownEl) {
    console.error('TimezonePicker: Required elements not found');
    return;
  }

  // Fetch timezone data
  this.fetchTimezones().then(function () {
    // Set initial value if provided
    if (self.initialValue) {
      self.setTimezone(self.initialValue);
    }
  });

  // Set up event listeners
  this.inputEl.addEventListener('input', function (e) {
    self.handleInput(e);
  });

  this.inputEl.addEventListener('focus', function () {
    self.showDropdown();
  });

  this.inputEl.addEventListener('keydown', function (e) {
    self.handleKeydown(e);
  });

  // Use event delegation for dropdown options
  // Using mousedown instead of click to ensure it fires before any potential interference
  this.dropdownEl.addEventListener('mousedown', function (e) {
    var option = e.target.closest('.tz-option');
    if (option) {
      e.preventDefault(); // Prevent focus loss
      var index = parseInt(option.getAttribute('data-index'), 10);
      self.selectTimezone(self.filteredTimezones[index]);
    }
  });

  this.dropdownEl.addEventListener('mouseover', function (e) {
    var option = e.target.closest('.tz-option');
    if (option) {
      var index = parseInt(option.getAttribute('data-index'), 10);
      if (self.selectedIndex !== index) {
        self.selectedIndex = index;
        self.updateSelectedClass();
      }
    }
  });

  // Close dropdown when clicking outside
  document.addEventListener('click', function (e) {
    if (!self.inputEl.contains(e.target) && !self.dropdownEl.contains(e.target)) {
      self.hideDropdown();
    }
  });
};

TimezonePicker.prototype.fetchTimezones = function () {
  var self = this;

  return fetch('/api/timezones')
    .then(function (response) {
      if (!response.ok) {
        throw new Error('Failed to fetch timezones');
      }
      return response.json();
    })
    .then(function (groups) {
      // Flatten the groups into a single array with region info
      self.timezones = [];
      groups.forEach(function (group) {
        group.timezones.forEach(function (tz) {
          self.timezones.push({
            id: tz.id,
            displayName: tz.display_name,
            offset: tz.offset,
            offsetMins: tz.offset_mins,
            region: group.region
          });
        });
      });

      // Initial filter shows all
      self.filteredTimezones = self.timezones.slice();
    })
    .catch(function (error) {
      console.error('TimezonePicker: Failed to fetch timezones', error);
    });
};

TimezonePicker.prototype.handleInput = function () {
  var query = this.inputEl.value.trim().toLowerCase();
  this.filterTimezones(query);
  this.showDropdown();
  this.selectedIndex = -1;
  this.renderDropdown();
};

TimezonePicker.prototype.filterTimezones = function (query) {
  var self = this;

  if (!query) {
    this.filteredTimezones = this.timezones.slice();
    return;
  }

  // Check if query matches any abbreviation
  var abbrevMatches = [];
  var upperQuery = query.toUpperCase();
  for (var abbrev in TIMEZONE_ABBREVIATIONS) {
    if (abbrev.indexOf(upperQuery) === 0) {
      abbrevMatches = abbrevMatches.concat(TIMEZONE_ABBREVIATIONS[abbrev]);
    }
  }

  this.filteredTimezones = this.timezones.filter(function (tz) {
    // Match against timezone ID (e.g., "America/New_York")
    if (tz.id.toLowerCase().indexOf(query) !== -1) {
      return true;
    }

    // Match against display name (e.g., "Eastern Time (US & Canada)")
    if (tz.displayName.toLowerCase().indexOf(query) !== -1) {
      return true;
    }

    // Match against city name extracted from ID
    var parts = tz.id.split('/');
    var city = parts[parts.length - 1].replace(/_/g, ' ').toLowerCase();
    if (city.indexOf(query) !== -1) {
      return true;
    }

    // Match against abbreviation results
    if (abbrevMatches.indexOf(tz.id) !== -1) {
      return true;
    }

    return false;
  });
};

TimezonePicker.prototype.handleKeydown = function (e) {
  if (!this.isOpen) {
    if (e.key === 'ArrowDown' || e.key === 'Enter') {
      this.showDropdown();
      e.preventDefault();
    }
    return;
  }

  switch (e.key) {
    case 'ArrowDown':
      e.preventDefault();
      this.moveSelection(1);
      break;
    case 'ArrowUp':
      e.preventDefault();
      this.moveSelection(-1);
      break;
    case 'Enter':
      e.preventDefault();
      if (this.selectedIndex >= 0 && this.selectedIndex < this.filteredTimezones.length) {
        this.selectTimezone(this.filteredTimezones[this.selectedIndex]);
      }
      break;
    case 'Escape':
      e.preventDefault();
      this.hideDropdown();
      break;
    case 'Tab':
      this.hideDropdown();
      break;
  }
};

TimezonePicker.prototype.moveSelection = function (direction) {
  var newIndex = this.selectedIndex + direction;

  if (newIndex < 0) {
    newIndex = this.filteredTimezones.length - 1;
  } else if (newIndex >= this.filteredTimezones.length) {
    newIndex = 0;
  }

  this.selectedIndex = newIndex;
  this.renderDropdown();
  this.scrollToSelected();
};

TimezonePicker.prototype.scrollToSelected = function () {
  var selectedEl = this.dropdownEl.querySelector('.tz-option.selected');
  if (selectedEl) {
    selectedEl.scrollIntoView({ block: 'nearest' });
  }
};

TimezonePicker.prototype.showDropdown = function () {
  if (this.timezones.length === 0) {
    return; // Data not loaded yet
  }

  this.isOpen = true;
  this.dropdownEl.style.display = 'block';
  this.renderDropdown();
};

TimezonePicker.prototype.hideDropdown = function () {
  this.isOpen = false;
  this.dropdownEl.style.display = 'none';
  this.selectedIndex = -1;
};

TimezonePicker.prototype.renderDropdown = function () {
  var self = this;
  var html = '';

  if (this.filteredTimezones.length === 0) {
    html = '<div class="tz-no-results">No matching timezones found</div>';
  } else {
    // Group by region for display
    var currentRegion = '';

    this.filteredTimezones.forEach(function (tz, index) {
      // Add region header if needed
      if (tz.region !== currentRegion) {
        currentRegion = tz.region;
        html += '<div class="tz-region-header">' + self.escapeHtml(currentRegion) + '</div>';
      }

      var isSelected = index === self.selectedIndex;
      var currentTime = self.getCurrentTimeInTimezone(tz.id);

      html += '<div class="tz-option' + (isSelected ? ' selected' : '') + '" data-index="' + index + '">';
      html += '<div class="tz-option-main">';
      html += '<span class="tz-name">' + self.escapeHtml(tz.displayName) + '</span>';
      html += '<span class="tz-time">' + currentTime + '</span>';
      html += '</div>';
      html += '<div class="tz-option-sub">';
      html += '<span class="tz-id">' + self.escapeHtml(tz.id) + '</span>';
      html += '<span class="tz-offset">' + self.escapeHtml(tz.offset) + '</span>';
      html += '</div>';
      html += '</div>';
    });
  }

  this.dropdownEl.innerHTML = html;
};

// Update selected class without re-rendering (for hover highlighting)
TimezonePicker.prototype.updateSelectedClass = function () {
  var options = this.dropdownEl.querySelectorAll('.tz-option');
  var self = this;
  options.forEach(function (option, i) {
    var index = parseInt(option.getAttribute('data-index'), 10);
    if (index === self.selectedIndex) {
      option.classList.add('selected');
    } else {
      option.classList.remove('selected');
    }
  });
};

TimezonePicker.prototype.getCurrentTimeInTimezone = function (tzId) {
  try {
    var now = new Date();
    var options = {
      timeZone: tzId,
      hour: '2-digit',
      minute: '2-digit',
      hour12: true
    };
    return now.toLocaleTimeString('en-US', options);
  } catch (e) {
    return '';
  }
};

TimezonePicker.prototype.selectTimezone = function (tz) {
  // Update input display
  this.inputEl.value = tz.displayName + ' (' + tz.offset + ')';

  // Update hidden input with actual timezone ID
  if (this.hiddenInputEl) {
    this.hiddenInputEl.value = tz.id;
  }

  // Hide dropdown
  this.hideDropdown();

  // Call callback
  this.onSelect(tz);
};

TimezonePicker.prototype.setTimezone = function (tzId) {
  var self = this;

  // Find the timezone in our list
  var tz = this.timezones.find(function (t) {
    return t.id === tzId;
  });

  if (tz) {
    this.inputEl.value = tz.displayName + ' (' + tz.offset + ')';
    if (this.hiddenInputEl) {
      this.hiddenInputEl.value = tz.id;
    }
  } else {
    // If not found in list, try to use the raw ID
    // This handles detected timezones that might not be in our curated list
    if (this.hiddenInputEl) {
      this.hiddenInputEl.value = tzId;
    }
    // Try to get a display name for it
    var displayName = tzId.replace(/_/g, ' ').split('/').pop();
    this.inputEl.value = displayName + ' (' + tzId + ')';
  }
};

TimezonePicker.prototype.getValue = function () {
  if (this.hiddenInputEl) {
    return this.hiddenInputEl.value;
  }
  return '';
};

TimezonePicker.prototype.escapeHtml = function (text) {
  var div = document.createElement('div');
  div.appendChild(document.createTextNode(text));
  return div.innerHTML;
};

// Export for use as a module or global
if (typeof module !== 'undefined' && module.exports) {
  module.exports = TimezonePicker;
}
